# Gemini 网页协议反代（OpenAI 兼容）· 本地开箱即用版

基于 [zexadev/gemini-web2api-go](https://github.com/zexadev/gemini-web2api-go) 编译适配的本地版。

## 它解决什么问题
- 把 Gemini 网页端（`gemini.google.com`）的对话接口，逆向成标准 **OpenAI 兼容** API（`/v1/chat/completions`）。
- **匿名、免 API Key、免 Google 账号** —— 绕开 Google Cloud Code Assist 的账号/地区 onboarding 限制。
- 对外暴露 OpenAI 协议，可直接接各种客户端 / agent / WorkBuddy。

## 模型
- `gemini-3.6-flash`（默认，全能）
- `gemini-3.5-flash-lite`（轻量）

## 前置条件
1. 本机有本地代理（Clash / v2ray 等），且能出网到 Google。
2. 本仓库 `start.bat` 默认走 `http://127.0.0.1:7897`，请改成你自己的代理端口。
3. **代理必须常开**，否则服务出不了网（502）。

## 快速开始（Windows）
直接双击 `start.bat` 即可启动，监听 `http://localhost:8083`。

手动启动（参数说明）：
```
gemini-web2api-go.exe --port 8083 --proxy http://127.0.0.1:7897 --api-key sk-local-test --admin-token admin123
```
- `--port` 监听端口
- `--proxy` 出网代理（必须能到 Google）
- `--api-key` 调用本服务时填的 Bearer key
- `--admin-token` 管理面板 `http://localhost:8083/admin` 的 token

## 当 API 用（OpenAI 兼容）
```
base_url = http://localhost:8083/v1
api_key  = sk-local-test
model    = gemini-3.6-flash
```
```bash
curl http://localhost:8083/v1/chat/completions \
  -H "Authorization: Bearer sk-local-test" -H "Content-Type: application/json" \
  -d '{"model":"gemini-3.6-flash","messages":[{"role":"user","content":"你好"}]}'
```
管理面板：`http://localhost:8083/admin`（token = admin123），可看请求日志、导入 Google cookie 解锁 pro / 思考 / 图像。

## 官方 API 直连（可选,推荐的生产通道）

在 `config.json` 配上官方 [AI Studio](https://aistudio.google.com) API key（或环境变量 `OFFICIAL_API_KEYS`,逗号分隔多把）：

```json
{
  "official_api_keys": ["AIza..."],
  "official_models": ["gemini-3.6-flash"],
  "official_model_map": { "gemini-3.6-flash": "gemini-3-flash" }
}
```

配好后这些模型的请求**优先走官方端点**（完全合规,不碰网页风控,不怕封号）,官方失败自动回落网页反代,客户端 base_url / key 不用改。官方模型名以 AI Studio 文档为准,用 `official_model_map` 对上;不配白名单时所有请求都会先试官方,官方不认的模型名会白付一个 RTT 才回落,建议配上。细节见 `docs/COMPARISON.md` 与 `docs/RISK.md`。

健康探测:`GET /healthz` 返回 200 即进程存活,给容器编排用。

## 接入 WorkBuddy
在 `C:\Users\<你>\.workbuddy\models.json` 加：
```json
{
  "models": [
    {
      "id": "gemini-3.6-flash",
      "name": "Gemini 3.6 Flash (本地反代)",
      "vendor": "OpenAI",
      "apiKey": "sk-local-test",
      "url": "http://localhost:8083/v1/chat/completions",
      "maxInputTokens": 128000,
      "maxOutputTokens": 8192,
      "supportsToolCall": true
    }
  ]
}
```
改完完全退出 WorkBuddy（含托盘）重开，新对话里选该模型即可。

## 限流（匿名模式）
单出口并发 5 / RPM 30 / RPH 80，超了返回 429。agent 狂刷请放慢节奏，或往面板导入 cookie 提额。

## 本地数据面板（额度监控）
双击 `dashboard.html` 即可打开一个零依赖的本地看板，实时显示：
- 并发 / RPM / RPH 用量卡（带红黄预警与「剩余次数」提示）
- 近 24h KPI（请求数、成功率、平均延迟、输入/输出 token）及按模型拆分
- 浏览器本地留存的用量趋势图（RPH / RPM / 并发三条线）

数据来自 `/admin/api/usage` 与 `/admin/api/stats`。注意：这些是**代理本地护栏**（防出口 IP 被 Google 拉黑），并非 Google 官方配额；Google 对匿名网页端的真实隐形限额此处看不到。

## ⚠️ 免责声明
逆向网页协议，仅限个人学习 / 自用，请勿用于生产流量，遵守 Google 服务条款。

## 来源
- 上游：https://github.com/zexadev/gemini-web2api-go
- 本仓库附带 Windows 预编译二进制 `gemini-web2api-go.exe`（基于上游 v4.27.0 编译）
