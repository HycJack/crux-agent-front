# AGENTS.md — Hermes Go

## Project Overview

Hermes Go is a minimal agent framework in Go. Single binary, zero external runtime dependencies.

**Core idea**: Skill is App, Agent is OS. Primitives (file/system) are built-in syscalls. Everything else is a pluggable skill.

## Architecture

```
core/                 — Framework internals
  types/              — Message, ToolCall, ToolSchema, Session
  config/             — YAML config loader
  llm/                — LLM provider interface + OpenAI impl + Router (fallback/retry)
  context/            — System prompt + token budget + trimming + memory injection
  agentloop/          — Reasoning loop (LLM → tool → execute → repeat)
  bus/                — In-process event bus (subscribe/emit/targeted/wildcard)
  skill/              — Skill interface (Init/Shutdown/Capabilities/Handle) + Simple embed
  primitives/         — 9 built-in tools (5 file + 4 system)
  store/              — SQLite (session CRUD, KV, FTS search)
  sandbox/            — Execution isolation (none + process)
  transport/          — Agent-to-agent communication (InProcess, extensible to A2A/ACP)
  registry/           — Agent registry (register/discover/search)
  observe/            — Tracing (spans) + Metrics (LLM/tools/latency)
  guardrails/         — Input/output validation, PII detection, rate limits
  prompt/             — Prompt template registry with variable substitution
  vector/             — Vector storage interface + in-memory implementation
  runtime.go          — Assembly: wires everything, CLI with commands

skills/               — Pluggable capabilities
  terminal/           — Shell command execution (exec tool)
  memory/             — Persistent memory via MEMORY.md / USER.md
  cron/               — Scheduled tasks (add/list/remove jobs)
  compaction/         — Context compression via LLM summarization
  websearch/          — Web search (DuckDuckGo) + URL fetching
  delegate/           — Cross-agent task delegation via Transport

cmd/hermes/           — CLI entry point
demo/                 — Self-contained demo (mock LLM, 6 scenarios)
```

## Components

### Built-in Primitives (always available)
- `read_file`, `write_file`, `patch_file`, `search_content`, `search_files`
- `get_time`, `get_os`, `get_cwd`, `get_env`

### Skills (loaded from config)
- `terminal` — exec
- `memory` — memory_save, memory_search, memory_list, memory_delete
- `cron` — cron_add, cron_list, cron_remove
- `compaction` — compact_messages
- `websearch` — web_search, fetch_url
- `delegate` — delegate_task, discover_agents

### Core Infrastructure
- **Event Bus** — synchronous pub/sub
- **Context Manager** — token budget, trimming, memory injection
- **Store** — SQLite with session/KV/FTS
- **LLM Router** — multi-provider with fallback + retry
- **Transport** — InProcess (extensible to A2A/ACP)
- **Registry** — agent discovery
- **Observability** — tracing + metrics
- **Guardrails** — input/output validation, PII, limits
- **Sandbox** — none (dev) / process (restricted paths/cmds)
- **Vector** — in-memory (extensible to Qdrant/pgvector)
- **Prompt Registry** — template management with variables

## CLI Commands

- `/quit` — Exit
- `/new` — New session
- `/list` — List saved sessions
- `/load <id>` — Load session
- `/memory` — Show memories
- `/stats` — Session stats + token budget

## Running

```bash
go build -o hermes ./cmd/hermes/
export OPENAI_API_KEY=sk-xxx
./hermes -config config.yaml

go test ./... -v     # 111 tests
go run demo/main.go  # 6 demo scenarios
go vet ./...         # lint
```

## Stats

- 50 Go source files, 6169 lines total
- 3877 lines implementation, 2292 lines tests
- 111 test cases (all passing)
- 23 packages
- 12MB single binary

## Dependencies

- `github.com/mattn/go-sqlite3` — SQLite
- `gopkg.in/yaml.v3` — YAML config
