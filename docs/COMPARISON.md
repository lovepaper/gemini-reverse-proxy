# 生态学习报告:同类反代项目对比与本项目吸收情况

> 2026-11-03 调研。范围不止 Gemini:GPT / 多模型网关一起看,目标是把生态里**经过验证的好设计**吸收进本项目,把**前赴后继踩坑的路**标出来不走。

## 一、调研对象

| 项目 | 赛道 | 核心做法 | 状态 |
|------|------|----------|------|
| zexadev/gemini-web2api-go(上游) | Gemini 网页反代 | cookie + batchexecute 逆向 | 活跃;4.27.0 功能与本仓持平,近期全是文档修正 |
| router-for-me/CLIProxyAPI | 多厂商 CLI 凭据代理 | **OAuth / 官方 API key 多账号轮询** | 活跃,生态最大 |
| HanaokaYuzu/Gemini-API(gemini_webapi) | Gemini 网页反代(Python) | cookie 后台自动续期并持久化 | 活跃 |
| xqdoo00o/chatgpt-to-api | GPT 网页反代 | 账号密码自动登录 + access_token 池每日轮换 + HAR 池过验证码 | 活跃 |
| LLM-Red-Team 系列(kimi-free-api 等) | 各家网页反代 | token 池 + 匿名 token 引导 + SSE 转换 | **整系列已 404 下架** |
| one-api / new-api | 通用 LLM 网关 | 渠道抽象 + 渠道级模型映射 + 加权重试 + 下游 key 配额 | 活跃,事实标准 |
| dwgx/WindsurfAPI(3.1k★) | 客户端凭据复用 | 把 Windsurf/Devin 桌面端模型转成三套兼容 API | 活跃 |
| CJackHwang/AIstudioProxyAPI | AI Studio 浏览器自动化 | camoufox 反检测浏览器驱动网页 | 活跃 |

## 二、吸收进本项目的(2026-11-03)

### 1. 官方 API key 池直连(学 CLIProxyAPI + one-api 的渠道思想)✅

CLIProxyAPI 最大的启示:**官方凭据 > 爬来的凭据**。它支持 Gemini API key / AI Studio / Vertex / OAuth 多账号 round-robin——合规、稳定、不怕风控。

本实现(`internal/app/official.go`):
- `config.json` 配 `official_api_keys`(也认 `OFFICIAL_API_KEYS` 环境变量,逗号分隔)
- 配了 key 的 `/v1/chat/completions` 请求**优先走官方 OpenAI 兼容端点**,key 原子 round-robin 轮询
- 官方失败(网络错 / 4xx / 5xx,流式未开始)自动回落网页反代,客户端无感;`official_only: true` 可关掉回落
- `official_models` 白名单 + `official_model_map` 模型名映射,避免官方不认的模型名白付 RTT
- 转发请求绑 `r.Context()`,客户端断开上游跟着取消
- 老部署没配 key 时行为零变化

### 2. /healthz 健康检查 ✅

生态里成熟项目都提供健康端点供编排探测。distroless 镜像没 shell,Dockerfile HEALTHCHECK 用不了,靠编排层探 `/healthz`。

## 三、看过但有意不学的(及原因)

| 做法 | 来源 | 为什么不学 |
|------|------|-----------|
| HAR 池过验证码 | chatgpt-to-api | 自动化过验证码 = 自动化绕过滥用防护,正是 Google 封号页点名的行为,做了等于加速封号(见 docs/RISK.md) |
| 反检测浏览器(camoufox) | AIstudioProxyAPI | 同上,对抗性伪装是猫鼠游戏里最先被针对的目标 |
| 从浏览器自动捞 cookie(browser-cookie3) | gemini_webapi | 方便是真方便,但它会诱导你把**主号** cookie 喂进来。宁手动、别顺手 |
| 整个 LLM-Red-Team 路线 | kimi-free-api 等 | 整系列已下架 404——这个生态位存活率极低,不该作为生产依赖,更别投入深度定制 |

## 四、值得后续考虑的(按优先级)

1. **下游多 key 管理**(学 chatgpt-to-api 的 api_keys.txt / one-api 的 token 体系):现在全服务只有一把 API key,分发给多人就得共享;加个 key 表(发 key、记账、可吊销)能把多人共用场景管起来。
2. **渠道抽象**(学 one-api):今天实现的官方直连本质上是"第二种渠道"。如果以后还要加 Vertex AI、OAuth 账号等上游,值得抽成统一的 channel 接口(每渠道自带挑凭据 / 失败计数 / 模型映射)。
3. **凭据定时续期心跳**(学 chatgpt-to-api 的每日 token 续期):cookie 池已有 rotate + mergeSetCookie,但可以加个低频后台"探活 + 提前续期",把失效发现在请求时而非探活时。注意:探活本身就是对上游的自动化访问,频率要克制。
4. **Deep Research 工作流**(学 gemini_webapi):建计划 → 轮询状态 → 取报告。实现成本高、使用频率低,优先级靠后。

## 五、一个反复出现的规律

这次调研里最扎眼的不是任何 feature,而是**存活率**:LLM-Red-Team 整系列没了,Pandora 一脉几经生死,Copilot 逆向服务(ninja 等)批量阵亡。网页逆向反代这个生态位的宿命就是"作者追、平台打、用户号陪葬"。所以本项目的正确姿势是:**网页反代保留当实验和兜底,生产流量走官方渠道**——这也是这次进化把官方 key 池做进去的原因。
