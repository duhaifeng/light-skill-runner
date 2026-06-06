# light-skill-runner 架构设计

一个轻量、快速、易编译分发的 AI Skill 执行引擎，使用 Go 实现。

## 1. 目标

- **轻量**：核心依赖少，产出单个静态二进制。
- **快速**：启动快、内存占用小；真正瓶颈在 LLM 网络往返。
- **易编译运行**：`go build` 一键产出，支持交叉编译，零运行时依赖。
- **可扩展**：LLM provider、工具、skill 来源均可插拔。

## 2. 什么是 Skill

一个 Skill 是一个目录，核心是 `SKILL.md`：

```text
example-skill/
├── SKILL.md          # frontmatter(name, description, ...) + 正文指令
├── scripts/          # 可选：可执行脚本/工具
└── resources/        # 可选：模板、参考文档、示例
```

`SKILL.md` 结构：

```markdown
---
name: example-skill
description: 一句话说明这个 skill 解决什么问题、何时使用
---

（正文：给模型看的自然语言指令，说明完成此类任务的步骤、注意事项、可用脚本等）
```

## 3. 核心理念：渐进式加载（Progressive Disclosure）

分三层加载，最大化节省 LLM 上下文：

1. **元数据层**：系统提示中只放每个 skill 的 `name + description`，让模型知道"有哪些技能、何时用"。
2. **指令层**：模型选定某 skill 后，才注入其 `SKILL.md` 完整正文。
3. **资源层**：正文中引用的脚本/文档，由模型通过工具按需读取，不一次性塞入。

## 4. 总体流程

```text
用户请求
   │
   ▼
[Loader]  扫描 skills/ 目录，解析每个 SKILL.md 的元数据
   │
   ▼
[Registry] 内存中维护所有 skill 的元数据与路径
   │
   ▼
[Selector] 仅用 name+description 让 LLM 判断使用哪个/哪些 skill
   │
   ▼
[Context Builder] 注入选中 skill 的正文 + 可用工具清单，组装消息
   │
   ▼
┌──────────────── Agentic Loop（Runner）────────────────┐
│  调用 LLM API → 返回 (文本 | 工具调用)                  │
│         │                                              │
│   有工具调用？──是──▶ [Tool Layer/Executor] 执行并回传  │
│         │                                              │
│         否                                             │
│         ▼                                              │
└──────── 产出最终结果（达到结束条件或最大轮数）──────────┘
```

## 5. 模块划分

| 模块 | 包路径 | 职责 | 关键点 |
|------|--------|------|--------|
| Loader | `internal/loader` | 扫描目录、解析 frontmatter、校验 | 元数据缓存；必填字段校验 |
| Registry | `internal/registry` | 维护已加载 skill | 按名查询、列出元数据 |
| Selector | `internal/selector` | 选择相关 skill | MVP：LLM 看元数据选；后续：向量检索 |
| Context Builder | `internal/context` | 实现渐进式加载、组装 prompt | 三层加载策略 |
| LLM Client | `internal/llm` | 统一 LLM 调用接口 + provider 实现 | provider 无关接口，屏蔽差异 |
| Tool Layer | `internal/tools` | 暴露给模型的工具集 | 工具注册表 + JSON Schema |
| Executor | `internal/executor` | 执行 skill 携带的脚本 | 超时、工作目录隔离 |
| Runner | `internal/runner` | 编排 Agentic Loop | 最大轮数、错误处理、日志 |
| CLI | `cmd/skill-runner` | 命令行入口 | flag / 环境变量配置 |

## 6. LLM 抽象（多 provider）

定义统一接口，首版实现 **OpenAI 兼容 provider**（一套实现可对接 OpenAI / DeepSeek / Ollama / vLLM 等），后续可加 Anthropic Claude。

```go
type Message struct {
    Role       string      // system | user | assistant | tool
    Content    string
    ToolCalls  []ToolCall  // assistant 发起的工具调用
    ToolCallID string      // tool 角色回传时对应的调用 id
}

type ToolSpec struct {
    Name        string
    Description string
    Parameters  map[string]any // JSON Schema
}

type ChatRequest struct {
    Messages []Message
    Tools    []ToolSpec
}

type ChatResponse struct {
    Message      Message // 含文本或 ToolCalls
    FinishReason string
}

type Client interface {
    Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}
```

配置（环境变量 / flag）：
- `LLM_BASE_URL`（默认 OpenAI 官方地址，可指向本地兼容服务）
- `LLM_API_KEY`
- `LLM_MODEL`

## 7. 工具集（第一版）

暴露给模型的工具：

- `list_skills()` — 列出可用 skill 元数据
- `load_skill(name)` — 读取某 skill 的完整正文（指令层加载）
- `run_script(path, args)` — 在沙箱中执行 skill 携带的脚本
- `read_file(path)` / `write_file(path, content)` — 读写工作区文件

每个工具携带 JSON Schema，注入到 `ChatRequest.Tools`，由 Runner 解析模型的工具调用并分发执行。

## 8. 执行与沙箱

- **第一版**：`os/exec` + `context.WithTimeout` 超时控制 + 限定工作目录，路径做规范化校验防止越界。
- **后续**：资源限制（CPU/内存）、权限白名单、容器或 WASM 级隔离。

## 9. 项目结构

```text
light-skill-runner/
├── cmd/skill-runner/main.go     # CLI 入口
├── internal/
│   ├── loader/
│   ├── registry/
│   ├── selector/
│   ├── context/
│   ├── llm/                     # base 接口 + openai 兼容实现
│   ├── tools/
│   ├── executor/
│   └── runner/
├── skills/                      # 本地 skill 目录
│   └── example-skill/SKILL.md
├── docs/architecture.md
├── go.mod
└── README.md
```

## 10. 分阶段路线图

- **阶段 1（MVP，已完成）**：Loader + Registry + OpenAI 兼容 Client + 基本 Agentic Loop + `run_script`/读写文件工具；跑通"发现 → 选择 → 执行一个 skill"。
- **阶段 2**：见下文「架构演进 v2」。
- **阶段 3**：更强沙箱（资源/权限限制）、并发执行多 skill、向量检索选择器。

---

# 架构演进 v2

在 MVP 基础上引入四项能力：统一提示词管理、Web/桌面 UI、运行透视（tracing）、易扩展的多 LLM provider。已确认选型：

- 前端：**Vite + React + TypeScript**
- 桌面：**Wails**（复用 Go 引擎与 React 前端）
- 配置：**YAML**（`gopkg.in/yaml.v3`）
- 透视：先做**内置**（JSONL 落盘 + Web 调用树/时间线），Langfuse 导出器后置
- **工具调用模拟层**：本版即做（应对本地模型无 function-calling）

## v2.0 分层总览

```text
   CLI        Web(浏览器)      Desktop(Wails, 后续)
    │             │                  │
    │         HTTP/SSE          (复用 web 前端)
    └──────┬──────┴──────────────────┘
           ▼
     Engine Facade  (engine.Run(ctx, Request) → 事件流 + 结果)
           │
   ┌───────┼───────────┬───────────┬──────────┐
 loader  prompt(模板)  llm(注册表)  tools    trace(贯穿全程)
 registry           executor     runner
```

新增引擎门面 `internal/engine`，统一三端入口，输出「事件流 + 最终结果」。

## v2.1 提示词统一管理

- 新建 `prompts/` 目录，提示词为 `text/template` 模板文件。
- 通过 `go:embed` 内嵌进二进制（保持单文件分发）；若磁盘存在同名文件则优先使用（免编译热改）。
- `internal/prompt` 提供：模板名常量、加载（embed + 磁盘覆盖）、按数据渲染。

```text
prompts/
├── system.tmpl          # 主系统提示（注入 skill 元数据列表）
├── skill_select.tmpl    # 选择器提示（后续）
└── tool_emulation.tmpl  # 本地模型工具调用模拟提示
```

## v2.2 配置系统（YAML）

`internal/config` 加载 `config.yaml`，环境变量可覆盖。

```yaml
llm:
  provider: ollama            # openai | ollama | llamacpp | ...
  base_url: http://localhost:11434/v1
  model: qwen2.5
  api_key: ""
trace:
  exporters: [file, stream]   # 可加 langfuse
  dir: ./traces
server:
  port: 8080
prompts_dir: ./prompts        # 留空则用内嵌
skills_dir: ./skills
workdir: .
```

## v2.3 LLM Provider 注册表 + 工具调用模拟层

- **注册表/工厂**：`llm.Register(name, factory)` + `llm.New(cfg)`；新增 provider = 新文件 + 一行注册，不动核心。
- 首批：`openai`（覆盖 OpenAI/DeepSeek/vLLM，以及 ollama、llama.cpp 的 `/v1`）、可选 `ollama` 原生。
- **Capabilities**：每个 provider 标记 `supportsTools` / `supportsStreaming`。
- **工具调用模拟层**：当 `supportsTools=false` 时，runner 走模拟路径——用 `tool_emulation.tmpl` 提示模型输出固定 JSON 的工具调用，引擎解析后执行并回灌结果。对本地小模型至关重要。

```go
type Client interface {
    Chat(ctx, req) (ChatResponse, error)
    Capabilities() Capabilities
}
type Capabilities struct{ SupportsTools, SupportsStreaming bool }
```

## v2.4 运行透视（Tracing，借鉴 Langfuse）

`internal/trace`：

- **数据模型**：`Trace`（一次运行：输入、最终输出、总耗时、总 token）→ 嵌套 `Span`（类型：`skill-select` / `generation` / `tool-call`，含 input/output/timing/metadata/status/tokenUsage）。
- **导出器（可插拔，可并存）**：
  1. `file`：`./traces/<runId>.jsonl`，零依赖离线回看。
  2. `stream`：通过 SSE 实时推送给 Web UI。
  3. `langfuse`（后续）：HTTP 推送到 Langfuse。
- **打点**：`Tracer` 经 `context` 传递；runner 中「整轮=trace、每次 LLM 调用=generation span、每个工具调用=tool span」。

## v2.5 Web 服务与 API

`cmd/skill-server`（或 `skill-runner serve` 子命令）+ `internal/server`，前端静态资源经 `go:embed` 内嵌 → 仍单文件。

```text
GET  /api/skills              列出 skill
POST /api/runs               发起执行，返回 runId
GET  /api/runs/{id}/events   SSE：token 输出 / 工具调用 / trace span
GET  /api/runs               历史 run 列表
GET  /api/runs/{id}          某次 run 的完整 trace
```

## v2.6 前端（React）与透视视图

- `web/`：Vite + React + TS，构建产物嵌入 Go。
- 核心页面：执行输入框 + 实时输出；**透视视图**＝调用树/时间线，展示每轮 loop、每次 LLM 生成的 prompt/响应/token、每个工具调用入参与输出、耗时与状态（内置迷你 Langfuse）。

## v2.7 桌面端（Wails，后续）

`desktop/` 为 Wails 工程，直接引用 `internal/` 引擎与 `web/` 前端，产出原生 Windows/macOS 二进制，前端零重写。

## v2.8 目录结构 v2

```text
cmd/
  skill-runner/     CLI
  skill-server/     Web/API 服务
internal/
  engine/           引擎门面（事件流 + 结果）
  loader/ registry/ runner/ executor/ tools/
  prompt/           模板加载（embed + 磁盘覆盖）
  llm/              接口 + provider 注册表 + 各实现 + 模拟层
  trace/            tracing 模型 + 导出器(file/stream/langfuse)
  config/           配置加载
  server/           HTTP + SSE
prompts/            提示词模板（内嵌）
web/                React 前端（构建产物内嵌）
desktop/            (后续) Wails
skills/  traces/  docs/
```

## v2.9 实施顺序

1. 配置系统（`config.yaml`）+ 提示词目录（地基）
2. Provider 注册表 + capabilities + 工具调用模拟层
3. trace 子系统 + JSONL 导出 + runner 打点
4. 引擎门面 + Web 服务 + SSE
5. React 前端 + 透视视图
6. （后续）Langfuse 导出器
7. （后续）Wails 桌面端

## v2.10 新增依赖

- `gopkg.in/yaml.v3`（配置解析）
- 前端：Node 工具链（Vite/React，仅开发期；产物内嵌）
- 后续：Wails CLI（桌面端）
