package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

// 官方 Gemini API key 池直连。
//
// 学的是 CLIProxyAPI / Gemini-Balance 这类项目的官方凭据池模式：官方 API key
// round-robin 轮询，配了 key 的请求优先走 Google 官方 OpenAI 兼容端点 ——
// 完全合规、不碰 gemini.google.com 的网页风控（cookie 反代的死穴，见 docs/RISK.md）。
// 官方失败（网络错 / 4xx / 5xx，且流式还没开始）自动回落网页反代路径；没配 key
// 时行为跟原来完全一样，老部署零感知。

// defaultOfficialBaseURL 是 Google 官方 OpenAI 兼容端点的 base URL。
const defaultOfficialBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"

// officialKeyIdx 是 round-robin 游标：每次发号 +1，按池子大小取模。
// 原子计数而不是锁 —— 挑 key 在每请求路径上，不需要互斥，只要并发下不重复就行。
var officialKeyIdx atomic.Uint64

// officialClient 按 (baseURL) 复用一个标准库 client。
//
// 不设整体超时：流式回复可以跑几分钟（跟网页路径 WriteTimeout=0 同理）。
// 用 Transport 层的连接/响应头超时兜底 —— 拨号连不上、TTFB 卡死这类故障
// 快速失败，好让请求回落网页路径，而不是吊死在官方端点上。
var officialHTTPClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	},
}

// pickOfficialKey 从 key 池轮询取一把。池子空返回 ok=false，调用方直接走原路径。
func pickOfficialKey() (string, bool) {
	keys := cfg.OfficialAPIKeys
	if len(keys) == 0 {
		return "", false
	}
	idx := officialKeyIdx.Add(1)
	return keys[(idx-1)%uint64(len(keys))], true
}

// officialEligibleModel 判断这个模型名该不该先试官方：白名单空 = 都试，
// 非空 = 只试名单内的。配白名单可以避免「官方不认的模型名白付一个 RTT」。
func officialEligibleModel(model string) bool {
	if len(cfg.OfficialModels) == 0 {
		return true
	}
	for _, m := range cfg.OfficialModels {
		if m == model {
			return true
		}
	}
	return false
}

// mapOfficialModel 把客户端的模型名映射成官方 API 认的名字；映射表里没有的原名透传。
func mapOfficialModel(model string) string {
	if mapped, ok := cfg.OfficialModelMap[model]; ok && mapped != "" {
		return mapped
	}
	return model
}

// tryOfficialUpstream 把 /v1/chat/completions 请求转发到官方 OpenAI 兼容端点。
//
// 返回 handled=true 表示响应已经写给客户端（成功透传，或 official_only 模式下的
// 失败报错），调用方直接 return。返回 false 表示官方这条路没走通**且还没写任何
// 字节**，调用方继续走网页反代路径。
//
// 响应体原样透传（官方本来就是 OpenAI 协议，不解析不翻译）：流式逐块带 flush，
// 非流式一次拷贝。转发请求绑 r.Context()，客户端断开时上游请求跟着取消。
func tryOfficialUpstream(w http.ResponseWriter, r *http.Request,
	rawBody []byte, modelInput, key string, stream bool, prompt string) bool {
	baseURL := cfg.OfficialBaseURL
	if baseURL == "" {
		baseURL = defaultOfficialBaseURL
	}

	// 只改 body 里的 model 字段，其余（messages/tools/temperature/...）原样转发。
	var req map[string]interface{}
	if err := json.Unmarshal(rawBody, &req); err != nil {
		return false // 不是合法 JSON，交给原路径自己报 400
	}
	req["model"] = mapOfficialModel(modelInput)
	out, err := json.Marshal(req)
	if err != nil {
		return false
	}

	upstream := baseURL + "/chat/completions"
	req2, err := http.NewRequestWithContext(r.Context(), "POST", upstream, bytes.NewReader(out))
	if err != nil {
		return false
	}
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+key)
	req2.Header.Set("Accept", "application/json")

	resp, err := officialHTTPClient.Do(req2)
	if err != nil {
		logf("[official] 请求失败，回落网页路径: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		// 官方拒绝（key 无效 / 模型不存在 / 配额尽）。 official_only 直接报错；
		// 否则静默回落 —— 网页路径会记自己的账，这里不重复记。
		snip, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		logf("[official] HTTP %d，回落网页路径: %s", resp.StatusCode, truncate(string(snip), 200))
		if cfg.OfficialOnly {
			writeJSON(w, resp.StatusCode, map[string]interface{}{
				"error": map[string]string{
				"message": "official API upstream error: HTTP " +
					strconv.Itoa(resp.StatusCode) + " " + truncate(string(snip), 200),
					"type": "upstream_error",
				},
			})
			return true
		}
		return false
	}

	// 200：透传响应。到这一步流可能已经开始 —— 不能再回落了，只能透传到底。
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	buf := make([]byte, 32*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				logf("[official] 客户端断开，停止透传: %v", werr)
				return true
			}
			rc.Flush()
		}
		if rerr != nil {
			break
		}
	}
	recordRequest("official.chat.completions", modelInput, prompt, "", nil, 200, "", stream)
	return true
}
