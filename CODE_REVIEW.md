# crux-agent-front 代码审核报告

审核对象：`https://github.com/HycJack/crux-agent-front.git`（main，30 commits，最后提交 2026-06-01）
审核时间：2026-10-06　　审核方式：全量源码通读 + 构建/测试/静态检查实跑

---

## 一、总体结论

**这不是一个前端项目，而是一个「Agent 框架 + Go 后端 + React 前端」的全栈应用**，但仓库名叫 `crux-agent-front`，README 也只描述了一个 9 端点的 Demo —— 文档与代码严重脱节。

| 区域 | 文件数 | 代码行 | 测试 |
|------|-------|--------|------|
| `backend/` (Gin) | 12 | 6,782 | ❌ 无 |
| `crux-agent-go/` (框架) | 83 | 12,182 | ✅ 27 个包全绿 |
| `frontend/src/` (React 19) | 17 | 5,655 | ❌ 无 |
| **合计** | **112** | **24,619** | 仅框架有测试 |

**核心判断**：这是一个功能相当完整的 AI Agent 平台雏形（多用户、JWT 鉴权、会话、团队、技能系统、飞书/Telegram/微信网关、多模型 Provider），但它处在一个**「原型能跑、无法上线」**的状态：

1. 🔴 **有一个明文 API Key 提交进了公开仓库** —— 必须立即吊销
2. 🔴 **LLM 拥有宿主机无沙箱 shell 权限**，且默认 Agent 全量授权
3. 🔴 **多租户越权（IDOR）系统性缺失**
4. 🟠 **克隆后无法编译**（go.mod 硬编码了作者机器路径）
5. 🟠 **应用层 12k 行代码 0 测试**，前端 lint 直接失败

值得肯定的是：密码用 bcrypt、SQL 全部参数化、JWT 密钥强制要求配置、SSE 解析器写得相当规范。问题集中在**安全边界设计**和**工程化**两块，而不是基本功。

---

## 二、P0 —— 立即处理

### 1. 明文 API Key 已提交到公开仓库 🔴

**`crux-agent-go/config.yaml:11`**
```yaml
llm:
  provider: xiaomi
  model: mimo-v5-pro
  api_key: tp-c4ds6kas5223hrfg0q64ao5m1hkr8q6886e7fpaevjlbr8cw
  base_url: https://token-plan-cn.xiaomimimo.com/v1
```

已验证：
- `git ls-files` 确认被 git 跟踪，**未被 .gitignore 覆盖**
- 引入自 commit `059533a "Add hermes-go framework"`
- 远端 `https://github.com/HycJack/crux-agent-front` 返回 **HTTP 200，即公开可访问**
- 全仓扫描，这是**唯一**一处密钥

**处置**：① 立即在小米 MiMo 控制台吊销并轮换该 key；② 从 git 历史中清除（`git filter-repo` 或 BFG）；③ `config.yaml` 加入 `.gitignore`，仓库内只保留 `config.yaml.example`。

> 注：泄露的 key 属于作者的 MiMo token-plan 额度。若该 key 仍有效，攻击者可无限消耗作者的模型额度；若历史会话中曾用此 key 跑过真实数据，那些数据也应视为已泄露。

### 2. 未认证 → 宿主机任意命令执行 🔴

完整攻击链（每一环都已亲自验证）：

| # | 环节 | 位置 | 事实 |
|---|------|------|------|
| 1 | exec 工具 = 裸 shell | `crux-agent-go/skills/terminal/skill.go:85` | `exec.CommandContext(ctx, "bash", "-c", command)`，`command` 是 LLM 传入的工具参数 |
| 2 | 无命令白名单 | `skill.go:19` | `AllowedCmds` 默认为空 = 全部放行 |
| 3 | 文件沙箱包含整个 `$HOME` | `backend/main.go:793` | `primitives.NewFileSandbox([]string{workDir, home, "/tmp"})` |
| 4 | exec 无条件注册 | `backend/main.go:797-800` | `terminal.New()` 直接注册，不受任何开关控制 |
| 5 | **默认 Agent 拥有全部工具** | `backend/main.go:854` | `Tools: engine.toolNames()` —— 含 `exec` / `fetch_url` / `get_env` / `cron_add` |
| 6 | 未指定 agent 时回落到默认 | `backend/main.go:1636-1638` | 新用户第一次发消息即命中默认 Agent |

**即：任何注册用户 → `POST /api/chat`（不带 agent_id）→ 宿主机任意命令执行，无需任何提权。**
可读取 `~/.ssh/id_rsa`、`~/.aws/credentials`、`.env`、数据库，并通过 `fetch_url` 外传。

**"沙箱"是不存在的** —— 这是最需要澄清的一点：

- `core/sandbox` 包**从未被任何非测试代码 import**（已 grep 确认），且其自身实现 `core/sandbox/process.go:110` 也是 `exec.Command("bash", "-c", cmd)`，只做 `strings.Contains` 字符串匹配，网络策略直接 `return nil`
- `core/guardrails` 包**同样从未被 backend import**。它就算接上，也只是 3 个可绕过的子串黑名单（默认 `{"rm -rf /", "dd if=", "mkfs"}`）—— `rm -fr /` 即可绕过
- `core/sandbox/none.go:17-21` 五个方法全部 `return nil`，是完全透传

也就是说：**框架看起来有安全层，实际是死代码；仓库文档 `AGENTS.md` 对沙箱能力的描述与二进制里的真实行为不符。**

### 3. 网关账号硬编码密码 + 首个用户自动成为管理员 🔴

两处独立代码叠加形成提权路径：

```go
// backend/gateway.go:309-310  —— 通过飞书/Telegram/微信发消息即可创建真实账号
username := platform + "_" + platformUserID        // 攻击者可控
user, _ := gm.engine.userStore.Create(username, "gateway-no-login", userName)
```
```go
// backend/db_stores.go:360-365  —— 库里第一个用户自动是 admin
role := "user"
var userCount int
s.db.QueryRow("SELECT COUNT(*) FROM app_users").Scan(&userCount)
if userCount == 0 { role = "admin" }
```

**利用**：在网站完成注册之前，先给飞书机器人发一条消息 → 系统自动创建 `feishu_<你控制的ID>` 账号 → 该账号是 user #0，**自动获得 admin** → 用公开已知的密码 `gateway-no-login` 通过 `POST /api/auth/login` 登录 → 完整管理员权限 + 全部工具的 shell 权限。

密码虽是 bcrypt 存储，但**明文常量就写在源码里**。

### 4. 飞书 Webhook 无签名校验，且校验逻辑可绕过 🟠→🔴

`backend/main.go:1290` 路由挂在 `gwAPI`（在 `auth` 组之外，公开）：
```go
gwAPI.POST("/feishu/webhook", gwManager.HandleFeishuWebhook)
```

`backend/feishu_adapter.go:123-130` 的 token 校验：
```go
if a.verificationToken != "" {
    var raw map[string]any
    if json.Unmarshal(body, &raw) == nil {
        if tok, ok := raw["token"].(string); ok && tok != a.verificationToken {
            c.JSON(403, gin.H{"error": "invalid verification token"})
            return
        }
    }
}
```
**只在 token 存在且不匹配时拒绝。请求体里不带 `token` 字段 → `ok == false` → 直接放行。**
且 `verificationToken == ""`（默认）时整个校验块被跳过。无速率限制，最终 `go a.manager.ProcessMessage(msg)` 直达 Agent Loop。

**影响**：网关一旦启用（`gwManager.Start()` 在启动时自动启动已启用的平台），未认证攻击者即可通过飞书触发 LLM + shell。属于 pre-auth RCE。

---

## 三、P1 —— 上线前必须修

### 5. 多租户越权（IDOR）系统性缺失

共 61 条路由，仅 3 条校验 `Role == "admin"`。多个 handler 缺失同族 `Update`/`Delete` 已有的归属校验：

| 端点 | 问题 | 位置 |
|------|------|------|
| `GET /api/agents/:id` | 任意用户可读他人 Agent 的 system prompt + 工具配置。`UpdateAgent` 有校验，`Get` 没有 | `main.go:1551-1558` |
| `GET /api/teams/:id` | 同上 | `main.go:2131-2138` |
| **5 个 runtime-skills 端点** | `ListRuntimeSkills` 调 `store.List()` 返回**全部用户**数据；`UserID` 字段建表时写了（`db_stores.go:102`）、创建时也写了（`:874`），**读取时从不按它过滤** | `skill_engine.go:586-667` |
| `POST /api/skills/:id/pin`、`/archive` | 无归属校验，任何人可 pin/归档他人技能 | `main.go:2682-2710` |
| `POST /api/curator/run` | 无 admin 校验，且遍历全部用户的 skill | `main.go:2668-2680` |
| `POST /api/chat/cancel` | 查 `activeCancels[sessionID]` 不校验 userID，任意用户可中断他人正在跑的 LLM 流 | `main.go:2428-2444` |

**修复方向**：在 store 层强制 `WHERE user_id = ?`，不要在 handler 层补 —— 目前"有的校验有的不校验"的分歧模式正是漏网的根因。

### 6. 网关 Bot Token 明文返回给任意登录用户 🟠

`backend/gateway.go:464` 直接把整个 `PlatformConfig` 塞进响应：
```go
c.JSON(200, gin.H{"platform": platform, "config": cfg})
```
其中 `Settings map[string]string`（`gateway.go:217-237`）持有 `bot_token` / `app_secret` / `verification_token` / `session_key`。该路由**仅要求登录，无 admin 校验**。

对比：模型 Provider 的 API Key 有专门的 `maskAPIKey()`（`main.go:2483-2488`）—— 同样的纪律在网关这块完全缺席。

同一批密钥还被以 `0644` 权限落盘（`gateway_config.go:53`），`OPENAI_API_KEY` 同样以 `0644` 写入（`main.go:342`）。

### 7. SSRF：`fetch_url` 无任何校验 🟠

`backend/skill_websearch.go:148-163`：
```go
u, _ := args["url"].(string)          // 完全由 LLM 指定
req, _ := http.NewRequest("GET", u, nil)  // 无 scheme / host / IP 校验
resp, err := w.client.Do(req)
```
无私有地址段过滤、无 `169.254.169.254` 云元数据屏蔽、无重定向限制。响应体（上限 200KB）原样回灌给模型 → 云凭据可被直接读出。

框架侧同源问题：`crux-agent-go/skills/websearch/skill.go:137` 同样是裸 `client.Get(u)`（目前该 skill 未注册，属潜伏问题）。

### 8. 克隆后无法编译 🟠

`backend/go.mod` 最后一行：
```
replace github.com/hermes-go => /root/hermes-go
```
作者机器上的绝对路径。实跑验证：
```
$ go build ./...
main.go:26:2: github.com/hermes-go@v0.0.0: replacement directory /root/hermes-go does not exist
```
**但框架源码就在仓库里**（`crux-agent-go/`，`module github.com/hermes-go`）。已验证改成 `../crux-agent-go` 后**编译通过**：

```bash
# 修复方式
cd backend && sed -i '' 's|/root/hermes-go|../crux-agent-go|' go.mod
$ go build ./...   # ✅ exit 0
```

### 9. `.env.example` 缺少必填项，新用户按 README 走必崩 🟠

代码中读取但 `.env.example` 未列出的变量：
- **`JWT_SECRET`** —— `main.go:60-63` 未配置时直接 `log.Fatalf` 退出。README 的快速启动步骤必然失败
- `CORS_ORIGINS` —— 未列出

**新用户完整失败链**：`go run .` 编译失败 → 改好 replace 后 `JWT_SECRET required` 退出 → 再配置后前端 3000 端口。README 全程没有提到鉴权、团队、技能、网关中的任何一项。

---

## 四、P2 —— 工程化与质量

### 10. 应用层 12,437 行代码，0 测试

框架 `crux-agent-go` 有 27 个测试包、全部通过（实跑确认，`go vet` 也干净）。但**真正承载用户数据和鉴权的 `backend/`（6,782 行）和 `frontend/`（5,655 行）一行测试都没有**。测试覆盖率的分布恰好与风险分布相反。

### 11. 前端 lint 直接失败

实跑 `npm run lint`：**24 errors, 4 warnings**
```
13  no-unused-vars
 8  react-hooks/set-state-in-effect
 4  react-hooks/exhaustive-deps
 2  react-hooks/immutability
 1  react-refresh/only-export-components
```
`npm run build` 成功（552ms），但主包 **1.62 MB**（gzip 454KB），无代码分割。

> `eslint.config.js` 本身**是正确的** —— 已核对 `eslint-plugin-react-hooks@7.1.1` 与 `react-refresh@0.5.2` 的实际导出形态，不存在版本不匹配。是代码本身没 lint 干净。

### 12. 三个已确认的前端功能缺陷

| 缺陷 | 位置 | 验证 |
|------|------|------|
| **图片上传按钮必然抛异常** | `Chat.jsx:844` `document.querySelector('.dropzone input').click()` | 全前端 grep：`.dropzone` 只在这一行出现。`getRootProps()` 挂在 `<div className="chat-layout">` 上（`:668`），不存在该 class → `null.click()` → TypeError。图片功能无法启动 |
| **SSE 解析器 buffer 污染** | `Chat.jsx:555` `buffer = lines.pop()` | 缺少 `api.js:62` 里有的 `\|\| ''` 兜底。分片不含 `\n` 时 `pop()` 返回 `undefined`，下一轮 `buffer +=` 会把字符串 `"undefined"` 拼进流里 |
| **写死一套更好却没人用的 SSE 实现** | `api.js:32-132` vs `Chat.jsx:535-618` | `sseConnect` 已 grep 确认**零调用方**。`api.js` 版本有 `res.ok` 校验、`\|\| ''` 兜底、AbortError 处理；`Chat.jsx` 内联版本三个全丢 —— 上面那个 buffer bug 正是因为绕过了好实现才存在 |

### 13. 提交了 46MB 二进制文件与垃圾文件

- `backend/crux-chat`（35MB）、`crux-agent-go/hermes`（12MB）—— `.gitignore` 只忽略了 `hermes-chat-backend`，没覆盖这两个。commit `f0949d0` 声称"remove binary files from git"，实际漏了
- `main.go.reference`（76KB）—— **语法非法的 Go 文件**，是某次编辑事故的残留（import 语句中间夹着 `result = e.executeTool(tc, extraTools)`）。`gofmt -e` 报错 `expected 'package', found result`
- 3 个 `.DS_Store`（`crux-agent-go/` 下）

仓库 .git 目录 128MB，其中绝大部分是这些二进制。

### 14. README 严重失真

README 描述 9 个 API 端点、称"9 tools"和"全部 14 个"（自相矛盾），代码实际 **61 条路由**。

**代码里有、README 完全没提的功能**（11 项）：登录鉴权、团队协作、技能 Curator、消息网关、飞书 / Telegram / 微信接入、邀请码体系、Runtime Skills、模型 Provider、cron 定时任务、上下文压缩。

### 15. 框架 83 个文件，约 45% 不可达

backend 实际只 import 6 个包：
```
core/llm, core/primitives, core/store, core/types, skills/memory, skills/terminal
```
`core/guardrails`、`core/sandbox`、`core/vector`、`core/skin`、`core/observe`、`core/plugin`、`core/cron`、`core/display`、`core/transport`、`harness/`（1,500+ 行）全部无生产调用方。

这些死代码里还埋着若干定时炸弹，一旦启用就会爆：
- `core/environment/env.go:203,208` —— `cat %s` 拼进 ssh argv，**远端 shell 注入**
- `core/llm/router.go:81-96` —— `RetryProvider` 缺 `Stream` 方法，不满足 `Provider` 接口，`WithRetry` 返回值不可用
- `core/transport/transport.go:84-89` —— `SendTask` 忽略 ctx，delegate 的 timeout 参数完全失效
- `core/cron/scheduler.go:154-161` —— `Stop()` 永久关闭 channel，再次 `Start()` 后调度器静默不再触发

---

## 五、并发与正确性（后端）

无 `panic`/`recover` —— `gin.Default()` 的 Recovery 只保护 handler goroutine，而项目里有大量裸 goroutine，**其中任何一个 panic 都是进程崩溃**。

| 问题 | 位置 | 严重度 |
|------|------|--------|
| **SSE ResponseWriter 并发写** | `main.go:1740` 启动 title goroutine → `:2381` 调 `sseSend`，与主 handler 同时写同一个 writer。每次新建 session 必现 | 🔴 真数据竞争 |
| title goroutine 在两条提前 return 分支上逃逸 | `main.go:1745-1755` 在 `titleWg.Wait()`（`:1760`）之前就 return，goroutine 继续往已结束的请求写数据 | 🔴 |
| 工具 goroutine 超时后**不取消，只是放弃等待** | `main.go:2069-2076`，注释 `:2061-2064` 自己承认。shell 进程继续跑。单请求最多泄漏约 10 个（maxRounds 默认 10） | 🟠 |
| `cron_add` 无上限起 goroutine | `skill_cron.go:141`，不持久化、不限流、不按用户隔离。而 `executeJob`(`:206-228`) **只是个打印日志的 stub** —— 纯泄漏无收益 | 🟠 |
| `runSubAgentSync` 收了 ctx 却不用 | `main.go:2298`，且内部 `executeTool` 同步执行**无超时**（对比主循环的 5 分钟预算 `:1809`），子 Agent 可永久挂住 SSE 连接 | 🟠 |
| 技能 ID 并发冲突 | `skill_manage.go:146` 无锁读 `len(map)` 生成新 ID，两个并发创建会撞 ID，后者静默覆盖前者 | 🟡 |
| `updateSkill` 改了但**从不落盘** | `skill_manage.go:172-191` 直接改 live map 指针，无锁、无 `save()`，还返回成功。LLM 被告知"更新成功" | 🟡 |
| `rows.Err()` 全仓 0 次检查 | 遍历中途出错会静默截断结果 | 🟡 |
| SQLite 外键声明了但从未启用 | `crux-agent-go/core/store/sqlite.go:99` 未设 `PRAGMA foreign_keys=ON` → `ON DELETE CASCADE` 不生效，删 session 会永久残留孤儿消息 | 🟡 |
| 无 schema 版本管理 | `sqlite.go:111-133` 纯 `CREATE TABLE IF NOT EXISTS`，无 `user_version`，未来加列会静默失效到查询时才报错 | 🟡 |

**其它值得单独一提的**：
- `core/llm/openai.go:321-332` —— 流式 `tool_call` 丢弃了 `index` 字段，改用 `ID` 归并，`ID == ""` 时并入 `accToolCalls[last]`。OpenAI 官方之外的 provider（Azure/DeepSeek/Qwen/Ollama/vLLM）交错下发时会把 #0 的参数片段并进 #1，产生畸形 JSON
- `core/llm/openai.go:360-366` —— 收到 `finish_reason` 置 `Done: true` 后**没有 return**，继续扫描并往 channel 写；消费者按惯例在 `Done` 停止读取 → goroutine 永久阻塞 → `defer resp.Body.Close()` 永不执行（连接泄漏）
- `core/llm/openai.go:107-118` —— 重试策略把所有错误（含 400/401/404）都重试 4 次
- `core/llm/openai.go:82-84` —— 硬编码 `max_tokens: 4096, temperature: 0.7`，`config.yaml` 里配的值被解析但从不使用
- `core/context/manager.go:40-43` —— 记忆内容无消毒、无长度上限地拼进 **system prompt**，而 LLM 自己就能通过 `memory_save` 写入 → **一次注入永久生效、跨重启存活**，这是拿到 shell 权限后最持久的植入通道
- `skills/compaction/skill.go:60` —— 传字面量 `"compacted"` 当 `beforeID`，子查询恒返回 NULL → **压缩标记永远写不进去，压缩功能实际未生效**

---

## 六、做得不错的地方

避免只挑刺，以下是经核实确实到位的设计：

1. **SQL 注入防护到位** —— `db_stores.go` 全部 49 处查询均用 `?` 参数化，包括两处动态拼接（`:441`、`:941`）拼接的都是字面量片段，用户值一律走 `args`
2. **密码处理正确** —— bcrypt `DefaultCost`，无明文存储
3. **JWT 密钥强制配置** —— `main.go:60-63` 未配置 `JWT_SECRET` 直接 `log.Fatalf` 拒绝启动，不静默降级到弱默认值
4. **上传文件名服务端生成** —— `main.go:2471` 用 `UnixNano()+ext`，攻击者文件名永不落盘；且 `main.go:2400` 的路径前缀校验能挡住穿越
5. **SSE 解析器（api.js 版本）实现规范** —— `getReader()` + `TextDecoder({stream:true})` 正确处理了多字节字符跨分片、partial line 缓冲、`AbortController` 回收、`AbortError` 区分（可惜没被使用）
6. **Markdown 渲染默认安全** —— 未启用 `rehype-raw`，`javascript:` URL 被 react-markdown v10 默认 `urlTransform` 拦截，KaTeX 未开 `trust`
7. **前端 API base 走环境变量** —— `import.meta.env.VITE_API_BASE || ''`，未硬编码 localhost
8. **框架工程质量高** —— 12,182 行代码 `go vet` 干净，27 个测试包全绿（含 sandbox/agentloop/llm 等高风险路径）
9. **React Router v7 用法正确** —— v6 风格 API 在 v7 仍受支持，无废弃 API；`ProtectedRoute` 守卫正常
10. **图片 Object URL 在 drop/手动移除路径上正确 revoke**（`Chat.jsx:467,838`）—— 只是发送路径漏了

---

## 七、建议修复顺序

**第 0 天（止血）**
1. 吊销并轮换 `config.yaml:11` 的 API Key；用 `git filter-repo` 从历史清除；`config.yaml` 进 `.gitignore`

**第 1 周（消除 Critical）**
2. `exec` 工具改为 argv 数组 + 显式命令白名单，或直接下线；至少加"人工确认"闸门
3. `core/sandbox` / `core/guardrails` 要么真正接线并加固，要么从 `AGENTS.md` 移除相关能力描述
4. 修复 `gateway.go:310` 硬编码密码（改为随机不可登录态账号，或标记 `LoginDisabled`）
5. 修复飞书 webhook 校验：改为"token 缺失即拒绝" + 启用 `encrypt` 签名校验
6. store 层统一加 `WHERE user_id = ?`；网关/curator/runtime-skills 端点加 admin 校验
7. `fetch_url` 加私有地址段与云元数据 IP 黑名单

**第 2 周（可构建 + 可验证）**
8. 修 `go.mod` replace 路径为 `../crux-agent-go`（已验证一行解决）
9. `.env.example` 补 `JWT_SECRET` / `CORS_ORIGINS`，README 补鉴权与鉴权启动步骤
10. 删掉 `main.go.reference`、两个二进制、3 个 `.DS_Store`
11. 前端：修 `.dropzone` 按钮、让 `Chat.jsx` 复用 `api.js` 的 `sseConnect`、跑通 `npm run lint`
12. 补 backend 集成测试（鉴权 + 越权 + SSE 三个场景即可覆盖最高风险面）

**第 1 个月（结构性）**
13. 拆 `main.go`（2,717 行 / 99 函数 / 42 handler）
14. 清理框架 45% 不可达代码，或补上依赖它的应用
15. 建立设计 token 层（当前 `App.css` 有 421 处硬编码 px、仅 11 个 CSS 变量），统一到一套断点
16. 补无障碍基础（当前全仓 **`aria-label` 数量为 0**）

---

## 附：审核方法说明

- 三个区域（backend / crux-agent-go / frontend）分别独立通读全部源码，再交叉核对
- 关键结论均**实跑验证**：`go build`（修复前失败/修复后成功）、`go vet`、`go test ./...`（27 包全绿）、`npm install`、`npm run lint`（24 errors）、`npm run build`（成功，1.62MB）
- 跨层验证：前端调用的 27 个端点与后端 61 条路由逐一比对，发现 `/api/invite-codes` 前端调用了但后端只有 `/api/invite-code`
- 所有 Critical/High 结论均已由审核者亲自复读源码确认，未直接采信子结论
- `go.mod` 替换路径的修复方案在临时目录中实测通过，未修改本仓库任何文件
