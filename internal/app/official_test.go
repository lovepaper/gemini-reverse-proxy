package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPickOfficialKeyRoundRobin(t *testing.T) {
	old := cfg
	t.Cleanup(func() { cfg = old; officialKeyIdx.Store(0) })

	officialKeyIdx.Store(0)
	cfg.OfficialAPIKeys = []string{"k1", "k2", "k3"}
	got := map[string]int{}
	for i := 0; i < 6; i++ {
		k, ok := pickOfficialKey()
		if !ok {
			t.Fatal("池子非空却取不到 key")
		}
		got[k]++
	}
	for _, k := range []string{"k1", "k2", "k3"} {
		if got[k] != 2 {
			t.Errorf("key %s 轮询次数 = %d, 应为 2", k, got[k])
		}
	}

	cfg.OfficialAPIKeys = nil
	if _, ok := pickOfficialKey(); ok {
		t.Error("空池子应返回 false")
	}
}

func TestMapOfficialModel(t *testing.T) {
	old := cfg
	t.Cleanup(func() { cfg = old })

	cfg.OfficialModelMap = map[string]string{"gemini-3.6-flash": "gemini-2.5-flash"}
	if got := mapOfficialModel("gemini-3.6-flash"); got != "gemini-2.5-flash" {
		t.Errorf("映射失败: %s", got)
	}
	if got := mapOfficialModel("gemini-unknown"); got != "gemini-unknown" {
		t.Errorf("表外模型应透传, got %s", got)
	}

	cfg.OfficialModelMap = nil
	if got := mapOfficialModel("any"); got != "any" {
		t.Errorf("无映射表应透传, got %s", got)
	}
}

func TestOfficialEligibleModel(t *testing.T) {
	old := cfg
	t.Cleanup(func() { cfg = old })

	cfg.OfficialModels = nil
	if !officialEligibleModel("anything") {
		t.Error("白名单空应全部放行")
	}
	cfg.OfficialModels = []string{"gemini-3.6-flash"}
	if !officialEligibleModel("gemini-3.6-flash") {
		t.Error("白名单内应放行")
	}
	if officialEligibleModel("gemini-image") {
		t.Error("白名单外应拦截")
	}
}

// 官方上游 500 → 回落（不写响应、返回 false）；200 → 透传并 handled=true。
func TestTryOfficialUpstreamFallbackAndPassthrough(t *testing.T) {
	old := cfg
	t.Cleanup(func() { cfg = old })

	// ── 上游 500：应回落 ──
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"boom"}`))
	}))
	defer up.Close()
	cfg.OfficialBaseURL = up.URL

	body, _ := json.Marshal(map[string]interface{}{
		"model":    "gemini-3.6-flash",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	if tryOfficialUpstream(w, r, body, "gemini-3.6-flash", "k1", false, "hi") {
		t.Error("上游 500 应回落（handled=false）")
	}
	if w.Code != 200 {
		t.Errorf("回落时不应写响应, got code=%d", w.Code)
	}

	// ── 上游 200：透传 body 与 Content-Type ──
	want := `{"id":"x","choices":[{"message":{"role":"assistant","content":"ok"}}]}`
	up2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k1" {
			t.Errorf("上游收到的鉴权头不对: %q", r.Header.Get("Authorization"))
		}
		var got map[string]interface{}
		json.NewDecoder(r.Body).Decode(&got)
		if got["model"] != "gemini-2.5-flash" {
			t.Errorf("转发前模型名未按映射表改写: %v", got["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(want))
	}))
	defer up2.Close()
	cfg.OfficialBaseURL = up2.URL
	cfg.OfficialModelMap = map[string]string{"gemini-3.6-flash": "gemini-2.5-flash"}

	r2 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(string(body)))
	w2 := httptest.NewRecorder()
	if !tryOfficialUpstream(w2, r2, body, "gemini-3.6-flash", "k1", false, "hi") {
		t.Error("上游 200 应 handled=true")
	}
	if w2.Body.String() != want {
		t.Errorf("透传内容不符: %s", w2.Body.String())
	}
}

// official_only 模式下官方失败要直接把错误写给客户端，不再回落。
func TestTryOfficialUpstreamOfficialOnly(t *testing.T) {
	old := cfg
	t.Cleanup(func() { cfg = old })

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer up.Close()
	cfg.OfficialBaseURL = up.URL
	cfg.OfficialOnly = true

	body, _ := json.Marshal(map[string]interface{}{
		"model":    "gemini-3.6-flash",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	if !tryOfficialUpstream(w, r, body, "gemini-3.6-flash", "k1", false, "hi") {
		t.Error("official_only 失败应 handled=true（错误已写给客户端）")
	}
	if w.Code != 401 {
		t.Errorf("应透传官方状态码, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "official API upstream error") {
		t.Errorf("错误信息缺失: %s", w.Body.String())
	}
}
