# Hermes Chat

全栈 Chat 应用：Go/Gin 后端（集成 hermes-go agent）+ React 前端，支持 SSE 流式输出 + 图片上传 + Tool Calling + 多 Agent 配置。

## 架构

```
┌──────────────────────────────────────────────────────────────┐
│  React Frontend (:3000)                                      │
│  ├─ / (Agent List)    ← 管理多个 Agent                       │
│  ├─ /agents/new       ← 创建 Agent                           │
│  ├─ /agents/:id/edit  ← 编辑 Agent（system prompt, tools）   │
│  └─ /chat/:agentId    ← 与 Agent 对话                        │
└────────────────────┬─────────────────────────────────────────┘
                     │ SSE + REST
┌────────────────────┴─────────────────────────────────────────┐
│  Gin Backend (:8080)                                         │
│  ├─ POST /api/chat       ← SSE 流式 + Agent Loop             │
│  ├─ CRUD /api/agents     ← Agent 配置管理                    │
│  ├─ GET  /api/tools      ← 可用工具列表                      │
│  └─ POST /api/upload     ← 图片上传                          │
└────────────────────┬─────────────────────────────────────────┘
                     │
┌────────────────────┴─────────────────────────────────────────┐
│  hermes-go (agent framework)                                 │
│  ├─ LLM Provider    (OpenAI 兼容 + SSE 流式)                 │
│  ├─ Primitives      (9 tools: file + system)                 │
│  ├─ Terminal Skill  (exec)                                   │
│  ├─ Memory Skill    (save/search/list/delete)                │
│  └─ Store           (SQLite 会话持久化)                      │
└──────────────────────────────────────────────────────────────┘
```

## 快速启动

```bash
export OPENAI_API_KEY=sk-xxx

# 启动后端
cd backend && go run .

# 启动前端
cd frontend && npm install && npm run dev
```

打开 http://localhost:3000

## Agent 配置

每个 Agent 可独立配置：
- **Name** — 名称
- **Description** — 描述
- **Model** — 模型（留空用默认）
- **System Prompt** — 系统提示词（定义 Agent 人格和行为）
- **Temperature** — 温度 (0-2)
- **Max Tokens** — 最大输出 token
- **Max Rounds** — 最大 Tool Calling 轮次
- **Tools** — 启用的工具（可精确控制 Agent 能力）

### 预设 Agent 示例

| Agent | 用途 | 工具 |
|-------|------|------|
| Hermes | 通用助手 | 全部 14 个 |
| Code Helper | 编码 | read_file, write_file, patch_file, exec, search_* |
| Researcher | 研究 | read_file, search_content, search_files, memory_* |
| Writer | 写作 | read_file, write_file (无 exec) |

## API

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/chat` | 流式聊天 (SSE)，支持 agent_id |
| GET | `/api/agents` | 列出所有 Agent |
| POST | `/api/agents` | 创建 Agent |
| GET | `/api/agents/:id` | 获取 Agent 配置 |
| PUT | `/api/agents/:id` | 更新 Agent |
| DELETE | `/api/agents/:id` | 删除 Agent |
| GET | `/api/tools` | 列出可用工具 |
| POST | `/api/upload` | 上传图片 |
| GET | `/api/health` | 健康检查 |

## 技术栈

- **后端**: Go + Gin + hermes-go
- **前端**: React + Vite + react-router-dom + react-markdown
- **通信**: SSE (Server-Sent Events)
- **存储**: SQLite (会话) + JSON (Agent 配置)

## 配置项

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `OPENAI_API_KEY` | (必填) | API Key |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | API 地址 |
| `OPENAI_MODEL` | `gpt-4o` | 默认模型 |
| `LISTEN_ADDR` | `:8080` | 后端端口 |
| `UPLOAD_DIR` | `./uploads` | 上传目录 |
