# AGENTS.md

Go 1.26+ proxy server providing OpenAI/Gemini/Claude/Codex compatible APIs with OAuth and round-robin load balancing.

## Repository
- GitHub: https://github.com/router-for-me/CLIProxyAPI

## Commands
```bash
gofmt -w . # Format (required after Go changes)
go build -o cli-proxy-api ./cmd/server # Build
go run ./cmd/server # Run dev server
go test ./... # Run all tests
go test -v -run TestName ./path/to/pkg # Run single test
go build -o test-output ./cmd/server && rm test-output # Verify compile (REQUIRED after changes)
```
- Common flags: `--config <path>`, `--tui`, `--standalone`, `--local-model`, `--no-browser`, `--oauth-callback-port <port>`

## Config
- Default config: `config.yaml` (template: `config.example.yaml`)
- `.env` is auto-loaded from the working directory
- Auth material defaults under `auths/`
- Storage backends: file-based default; optional Postgres/git/object store (`PGSTORE_*`, `GITSTORE_*`, `OBJECTSTORE_*`)

## Architecture
- `cmd/server/` — Server entrypoint
- `internal/api/` — Gin HTTP API (routes, middleware, modules)
- `internal/api/modules/amp/` — Amp integration (Amp-style routes + reverse proxy)
- `internal/thinking/` — Main thinking/reasoning pipeline. `ApplyThinking()` (apply.go) parses suffixes (`suffix.go`, suffix overrides body), normalizes config to canonical `ThinkingConfig` (`types.go`), normalizes and validates centrally (`validate.go`/`convert.go`), then applies provider-specific output via `ProviderApplier`. Do not break this "canonical representation → per-provider translation" architecture.
- `internal/runtime/executor/` — Per-provider runtime executors (incl. Codex WebSocket)
- `internal/translator/` — Provider protocol translators (and shared `common`)
- `internal/registry/` — Model registry + remote updater (`StartModelsUpdater`); `--local-model` disables remote updates
- `internal/store/` — Storage implementations and secret resolution
- `internal/managementasset/` — Config snapshots and management assets
- `internal/cache/` — Request signature caching
- `internal/watcher/` — Config hot-reload and watchers
- `internal/wsrelay/` — WebSocket relay sessions
- `internal/usage/` — Usage and token accounting
- `internal/tui/` — Bubbletea terminal UI (`--tui`, `--standalone`)
- `sdk/cliproxy/` — Embeddable SDK entry (service/builder/watchers/pipeline)
- `test/` — Cross-module integration tests

## Code Conventions
- Keep changes small and simple (KISS)
- Comments in English only
- If editing code that already contains non-English comments, translate them to English (don’t add new non-English comments)
- For user-visible strings, keep the existing language used in that file/area
- New Markdown docs should be in English unless the file is explicitly language-specific (e.g. `README_CN.md`)
- As a rule, do not make standalone changes to `internal/translator/`. You may modify it only as part of broader changes elsewhere.
- If a task requires changing only `internal/translator/`, run `gh repo view --json viewerPermission -q .viewerPermission` to confirm you have `WRITE`, `MAINTAIN`, or `ADMIN`. If you do, you may proceed; otherwise, file a GitHub issue including the goal, rationale, and the intended implementation code, then stop further work.
- `internal/runtime/executor/` should contain executors and their unit tests only. Place any helper/supporting files under `internal/runtime/executor/helps/`.
- Follow `gofmt`; keep imports goimports-style; wrap errors with context where helpful
- Do not use `log.Fatal`/`log.Fatalf` (terminates the process); prefer returning errors and logging via logrus
- Shadowed variables: use method suffix (`errStart := server.Start()`)
- Wrap defer errors: `defer func() { if err := f.Close(); err != nil { log.Errorf(...) } }()`
- Use logrus structured logging; avoid leaking secrets/tokens in logs
- Avoid panics in HTTP handlers; prefer logged errors and meaningful HTTP status codes
- Timeouts are allowed only during credential acquisition; after an upstream connection is established, do not set timeouts for any subsequent network behavior. Intentional exceptions that must remain allowed are the Codex websocket liveness deadlines in `internal/runtime/executor/codex_websockets_executor.go`, the wsrelay session deadlines in `internal/wsrelay/session.go`, the management APICall timeout in `internal/api/handlers/management/api_tools.go`, and the `cmd/fetch_antigravity_models` utility timeouts

## Environment
- Windows cmd.exe defaults to GBK (CP936). Always prefix Bash commands with `chcp 65001 > nul &&` to avoid garbled output.
- Playwright MCP: registered as `stdio` type via `trae-cli mcp add-json`, runs `npx @playwright/mcp --headless --no-sandbox` locally

## Language
- 回复语言：中文（用户明确要求使用中文回复）

## Deployment (proxy-ai-model)
- **Backend (Go)**
  - Source: `C:\Users\Docker\vs-project\workspace\CLIProxyAPI`
  - Build: `go build -o cli-proxy-win-amd64.exe ./cmd/server`
  - Deploy target: `C:\Users\Docker\Desktop\Workspace\proxy-ai-model\cli-proxy-win-amd64.exe`
- **Frontend (React)**
  - Source: `C:\Users\Docker\vs-project\workspace\Cli-Proxy-API-Management-Center`
  - Build: `npm run build` (outputs to `dist/index.html`)
  - Deploy target: `C:\Users\Docker\Desktop\Workspace\proxy-ai-model\static\management.html`
- Copy ONLY the base binary (without `_run` suffix) to the release directory
- The watchdog manages `_run` files automatically — it copies the base binary as `cli-proxy-win-amd64_run.exe` and runs it
- To restart: kill the `cli-proxy-win-amd64_run.exe` process, watchdog will restart with the new base binary
- Do NOT create or copy `_run` files directly; do NOT start the watchdog yourself

## Packet Capture Analysis

### Whistle 环境
- Whistle 代理地址: `http://10.11.61.34:8899` (no TLS, plain HTTP)
- Web UI: `http://10.11.61.34:8899/#network`
- MCP 工具目录: `.mcp-tools/whistle/`（34 个工具）
- 状态检查: `mcp__whistle__getWhistleStatus`（确认 interceptHttpsConnects=true, enableHttp2=true）

### 抓包方法（通过 MCP invoke_tool）
1. **检查 Whistle 状态**: `invoke_tool("mcp__whistle__getWhistleStatus", {})`
2. **抓取拦截信息**: `invoke_tool("mcp__whistle__getInterceptInfo", {"url": "console.enterprise.trae.cn", "count": 20})`
   - `url` 支持正则表达式，如 `"trae.cn"` 匹配所有子域名
   - `count` 控制返回数量，默认可能不够，建议 20-50
   - `startTime` 可选，指定起始时间 ms
3. **返回数据格式**: JSON 数组，每个元素包含 id/startTime/ttfb/url/req/res
   - 请求/响应体以 **base64 编码**，需解码后分析
   - 大文件（>256KB）会被缓存到 `file-cache/` 目录

### 多 Agent 分析策略（处理大规模抓包数据）
由于单次抓包可达 400KB+，单 Agent 上下文无法承载，采用以下策略：

1. **并行抓取**: 同时启动 2 个 Agent，分别抓取 `console.enterprise.trae.cn` 和 `api.enterprise.trae.cn`
2. **分段提取**: 对已缓存的大文件（file-cache/*.md），每个 Agent 只提取关键字段，不读取完整 base64 body
3. **一个子 Agent 只处理一条请求**: 每条 llm_raw_chat 请求的 base64 body 解码后可达 100KB+，一个子 Agent 处理多条请求会导致上下文爆炸。必须为每条请求启动独立的子 Agent
4. **关键字段清单**:
   - 请求级: id, startTime, ttfb, url, req.size, res.statusCode, res.size
   - 请求头: Extra（含 session_id）, X-Ide-Function
   - 响应头: Server-Timing（含 tt_agw）, Content-Type
   - 请求体 base64 解码后搜索: config_name, model_name, conversation_id, session_id
   - 响应体 base64 解码后搜索: cache_read, cache_creation, cache_hit_rate, prompt_tokens, completion_tokens, first_token_ms, gateway_connect, preprocess_timing
5. **汇总分析**: 主 Agent 汇总各子 Agent 的摘要，生成完整时间线表格

### 子 Agent 实战模板（已验证有效）
每个子 Agent 处理一条 llm_raw_chat 请求，prompt 模板如下：

```
Read the file at {file-cache路径}.md

Find the {第N个} entry where "url" contains "llm_raw_chat".

Extract ONLY these fields from this single request:
1. id, startTime, ttfb
2. From req.headers.extra (JSON string, parse it): session_id, model_name, display_name
3. From res.headers: server-timing (extract tt_agw dur value)
4. From res.base64: base64 decode, search for "cache_read_input_tokens", "cache_creation_input_tokens", "prompt_tokens", "completion_tokens" in SSE event:token_usage
5. From res.base64 decoded SSE: search for "first_token_ms" or "platform_first_token_timing" in event:timing_cost

Return ONLY in this exact format:
#N | id=XXX | startTime=XXX | ttfb=XXX | session=XXX | model=XXX | cache_read=XXX | cache_creation=XXX | prompt_tok=XXX | completion_tok=XXX | hit_rate=XX.X% | tt_agw=XXX | first_token_ms=XXX

Calculate hit_rate = cache_read / prompt_tokens * 100.
Do NOT include raw base64 content.
```

**关键要点**：
- 用 `{第N个}` 明确指定处理哪条请求，避免子 Agent 混淆
- 要求返回固定格式的单行输出，便于主 Agent 汇总
- 明确禁止返回 base64 原始内容，防止上下文爆炸
- 所有子 Agent 并行启动，最大化效率
- 主 Agent 收到所有子 Agent 结果后，汇总成对比表格，分析 console vs api 的缓存命中差异

### 分析指标说明
| 指标 | 来源 | 含义 |
|------|------|------|
| TTFB | 顶层字段 | 首字节响应时间，低值可能表示缓存命中 |
| tt_agw | Server-Timing 响应头 | API 网关耗时，占 TTFB 86-90% |
| first_token_ms | SSE body | 平台首 token 延迟 |
| cache_read | SSE body | KV cache 命中读取的 token 数 |
| cache_creation | SSE body | 新写入缓存的 token 数（0=只读缓存） |
| cache_hit_rate | SSE body | cache_read / prompt_tokens |
| prompt_tokens | SSE body | 输入 token 总数 |
| completion_tokens | SSE body | 输出 token 数 |
| queue_timing | SSE body | 排队时间（0=无排队） |
| preprocess_timing | SSE body | 预处理耗时 |

### 如何判断 KV Cache 命中（关键字段位置）
KV cache 命中信息在 SSE 响应体的 `event: token_usage` 事件中，具体字段：
- **`cache_read_input_tokens`** — 从 KV cache 命中读取的 token 数（>0 表示有缓存命中）
- **`cache_creation_input_tokens`** — 新写入缓存的 token 数（0=只读缓存，无新写入）
- 命中率 = `cache_read_input_tokens / prompt_tokens`

在 Whistle 抓包中的提取路径：
```
res.base64 → base64 解码 → SSE 文本流 → event: token_usage → data JSON
```
示例 SSE 片段：
```
event: token_usage
data: {"prompt_tokens":27340,"completion_tokens":323,"cache_read_input_tokens":26624,"cache_creation_input_tokens":0,...}
```
- `cache_read_input_tokens=0` 且 `cache_creation_input_tokens=0` → 完全无缓存命中（冷启动）
- `cache_read_input_tokens>0` 且 `cache_creation_input_tokens=0` → 只读缓存命中（最常见）
- `cache_read_input_tokens>0` 且 `cache_creation_input_tokens>0` → 部分命中+新写入

## Trae Gateway Analysis & Findings (2026-05-25)

### KV Cache
- **结论：同一 session_id 复用显著提升 KV cache 命中率**
- glm-5v-turbo + session `6983be0e`: cache_hit_rate 从 54.4% 逐步攀升至 77.1%（~30次请求）
- cache_read 稳定在 ~27377 tokens，cache_write 始终为 0（只读缓存，无新写入）
- DeepSeek-V4-Pro + session `926e4bac`: cache_hit_rate 始终 0%（不同模型，独立 session）
- **pcid=0 是正常的**，真实客户端也不发送 pcid

### 风控问题
- **DeepSeek-V4-Pro 触发 Trae 网关风控拦截**（"We're sorry, your requests hit the new risk control status."）
- 现有 session 轮换机制有效：rotation=2 时成功绕过
- 轮换延迟约 1-2s，用户可感知到失败重试
- glm-5v-turbo 不受风控影响

### 连接复用
- **HTTP 连接池未生效**：所有连接 `reused=false idle=false`，每次新建 TCP+TLS
- 服务器返回 `Connection: close`，阻止 HTTP/1.1 keep-alive 复用
- 影响：每次请求额外 ~30ms（TCP+TLS 握手），但 gateway_connect 本身 ~3-3.5s 占主导

### 延迟分析
- gateway_connect（首字节响应前等待）：~3-3.5s（上游限制，无法优化）
- first_token_ms：glm-5v-turbo 1.8-8.4s，DeepSeek 6.3-8.8s
- 总耗时：glm 短请求 3-25s，DeepSeek 长请求可达 6m52s（8081 tokens）

### 已修复问题
- [x] 日志字段名：pf_tt/pf_ti/pfi → first_token_ms/first_token_idx/flush_idx
- [x] KV cache 字段：添加 CacheReadInputTokens / CacheCreationInputTokens
- [x] HTTP 客户端超时：移除 150s 限制（Timeout=0），支持长请求
- [x] 无超时错误：重启后未再出现 context deadline exceeded
- [x] Extra 字段补齐 `proxy_version:""`（真实客户端有此字段）

### 待优化项
1. 连接复用：让 HTTP 传输层复用连接（需确认能否绕过 Connection: close）
2. 风控重试：缩短重试间隔、增加重试次数，减少用户感知的延迟
3. session_id 策略：分析是否需要像真实客户端那样更频繁地轮换 session

### truncateMessages 已移除

**结论：CLIProxy 不应做消息截断。**

- `truncateMessages` 是 CLIProxy 自行添加的预防性措施，真实客户端（Claude Code/Trae IDE）不做截断，将完整对话历史原样发送
- 截断会破坏 KV cache 依赖的 prompt 前缀一致性，导致缓存失效
- 网关并未因消息数量拒绝请求，风控担心是多余的
- **已删除** `truncateMessages` 函数及两处调用点（`buildOpenAIGatewayRequest` 和 `buildAnthropicGatewayRequest`），消息原样透传

### 新任务：KV Cache 命中规律抓包分析
- **目标**：通过 MCP Server 抓包工具分析 KV cache 命中的规律
- **URL 区分**：
  - `console` 开头的 URL → 真实客户端请求
  - `api` 开头的 URL → CLIProxy 代理的请求
  - 两者地址不同但指向同一推理服务
- **策略**：采用多 Agent 机制进行分析，因为抓取的报文量非常大，单 Agent 上下文无法承载
- **状态**：进行中（console + api 流量均已捕获，持续分析中）

### KV Cache 完整分析（2026-05-27，3个抓包文件，同一 Session）

**Session**: `6221af46-80fb-4e71-8185-c3bb9aa8f5b8` | **Model**: deepseek-V4-Pro | **6 次 llm_raw_chat 请求**

| # | TTFB | cache_read | cache_hit | prompt_tok | completion | tt_agw | first_token |
|---|------|------------|-----------|------------|------------|--------|-------------|
| 1 | 1868ms | 26,624 | 97.4% | 27,340 | 323 | 1606ms | 1510ms |
| 2 | 1681ms | 26,624 | 95.7% | 27,833 | 472 | 1440ms | 1332ms |
| 3 | 2168ms | 38,912 | 95.0% | 40,972 | 159 | 1904ms | 1790ms |
| 4 | 2175ms | 40,960 | 95.6% | 42,859 | 314 | 1908ms | 1768ms |
| 5 | 2079ms | 40,960 | 93.9% | 43,613 | 220 | 1855ms | 1705ms |
| 6 | 2306ms | 43,008 | 98.1% | 43,846 | 264 | 2073ms | 1944ms |
| 7 | 3303ms | 40,960 | 94.1% | 43,529 | 174 | 3042ms | 2983ms |

**核心规律**：
1. **cache_read 阶梯式增长**：26,624 → 38,912 → 40,960(平台期) → 43,008，缓存窗口上限约 40K tokens (40×1024)
2. **命中率始终 93.9%-98.1%**，cache_creation 始终为 0（纯只读缓存，首次请求写入后不再更新）
3. **TTFB 与 prompt_tokens 正相关**：早期 27K tokens 时 TTFB ~1.7-1.9s，后期 40K+ tokens 时 ~2.1-2.3s
4. **网关占 TTFB 86-90%**（tt_agw 1.4-2.1s），是绝对瓶颈
5. **queue 始终为 0**：无排队压力
6. **api.enterprise.trae.cn 有流量**：CLIProxy 代理的请求已可捕获，与 console 请求共享同一批 session 数据

### Doubao_1_6 对比分析（2026-05-27，同一 conversation_id）

**conversation_id**: `6221af46-80fb-4e71-8185-c3bb9aa8f5b8` | **Model**: Doubao_1_6 (实际 `doubao-dev-1215-update2-dev`) | **2 次 llm_raw_chat 请求**

| # | TTFB | cache_read | cache_hit | prompt_tok | completion | tt_agw | first_token |
|---|------|------------|-----------|------------|------------|--------|-------------|
| 1 | 3282ms | - | - | - | - | 3040ms | - |
| 2 | 3349ms | **0** | **0%** | 196,074 | 2,101 | 3042ms | **36,103ms** |

**与 DeepSeek-V4-Pro 对比（同一 conversation_id）**：

| 指标 | DeepSeek-V4-Pro | Doubao_1_6 |
|------|----------------|-------------|
| cache_read | 26K→43K (阶梯增长) | **0** (始终) |
| cache_hit_rate | 93-98% | **0%** |
| prompt_tokens | 27K→43K | **196K** |
| first_token_ms | 1.3-1.9s | **36s** |
| tt_agw | 1.4-2.1s | ~3.0s |

**关键发现**：
1. **Doubao_1_6 完全无 KV cache 命中** — 每次请求都是冷启动，cache_read=0, cache_creation=0
2. **session_id 不复用** — 两次请求使用不同 session_id（`a3c1de3c` vs `76f9c091`），无法利用缓存
3. **prompt 巨大（196K tokens）** 导致首 token 延迟 36s，是 DeepSeek 的 20 倍
4. **请求 #1 被截断** — 响应仅 621 bytes，只有 progress_notice 无实际输出（可能被取消）
5. **网关延迟稳定** — tt_agw ~3s，与模型无关
6. **KV cache 按模型+session_id 隔离** — 同一 conversation 下不同模型/不同 session 不共享缓存

### 抓包对比分析（2026-05-25）
- **请求头层面完全一致**：CLIProxy 与真实客户端的 HTTP 请求头无差异
- **Extra JSON 差异**：CLIProxy 缺少 `proxy_version` 字段（已修复），其余字段一致
- **session_id 使用模式**：真实客户端同一模型固定 session 不轮换（glm ~30次不变，DeepSeek 也不变），CLIProxy 策略合理
- **请求体格式一致**：conversation_id / session_id / messages 格式一致
- **Accept-Encoding**：真实客户端显式发送 `gzip`，CLIProxy Go http.Client 自动处理
