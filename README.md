# light-skill-runner

一个轻量、快速、易编译分发的 **AI Skill 执行引擎**，使用 Go 实现，自带 Web 透视界面。

它扫描本地 skill 目录，借助 LLM 理解用户请求、选择合适的 skill，并通过工具调用真正执行任务；全过程可在内置 Web UI 中以调用树/时间线方式透视（借鉴 Langfuse）。

## 什么是 Skill

一个 skill 是一个目录，核心是 `SKILL.md`：开头的 frontmatter 描述元数据（`name`、`description`），正文是给模型看的自然语言指令。可附带脚本、模板等资源。

引擎采用**渐进式加载**：平时只把每个 skill 的 `name + description` 注入提示，模型选中后才加载完整正文，资源再按需读取，以节省上下文。

详见 [docs/architecture.md](docs/architecture.md)。

## 特性

- **轻量单二进制**：Go 实现，前端与默认提示词均内嵌，`go build` 即可分发。
- **多 LLM、易扩展**：provider 注册表 + capabilities；内置 `openai` / `ollama` / `llamacpp`（均走 OpenAI 兼容协议）。
- **本地模型友好**：对不支持原生 function-calling 的模型，自动启用提示词**工具调用模拟层**。
- **统一提示词管理**：`prompts/*.tmpl` 内嵌 + 磁盘可覆盖热改。
- **运行透视**：trace → 嵌套 span，JSONL 落盘 + SSE 实时推送 + Web 调用树视图。
- **多端复用**：CLI / Web 共用同一引擎；后续 Wails 桌面端可复用 Go 引擎与 React 前端。

## 目录结构

```text
cmd/
  skill-runner/   CLI 入口
  skill-server/   Web/API 服务入口
internal/
  engine/    引擎门面（统一入口，输出事件流 + 结果）
  loader/    扫描并解析 SKILL.md
  registry/  skill 注册表
  prompt/    提示词加载（内嵌 + 磁盘覆盖 + 模板渲染）
  llm/       LLM 接口 + provider 注册表 + OpenAI 兼容实现 + 能力标志
  tools/     暴露给模型的工具集
  executor/  脚本执行（超时 + 工作目录隔离）
  runner/    Agentic Loop（原生 / 模拟两种工具调用路径）
  trace/     运行透视：Trace/Span + 导出器(file/stream/console)
  config/    配置加载（默认值 < YAML < 环境变量）
  server/    HTTP + SSE
prompts/     提示词模板（内嵌）
web/         React(Vite+TS) 前端，dist 内嵌
skills/      本地 skill 目录
```

## 构建

需要 Go 1.22+。前端产物已内嵌（如需改前端见下文）：

```bash
go build -o skill-runner ./cmd/skill-runner
go build -o skill-server ./cmd/skill-server
```

## 配置

配置优先级：内置默认值 < `config.yaml` < 环境变量。示例见 [config.yaml](config.yaml)。

```yaml
llm:
  provider: openai            # openai | ollama | llamacpp
  base_url: https://api.openai.com/v1
  model: gpt-4o-mini
  api_key: ""
  force_tool_emulation: false # 本地模型无 function-calling 时设 true
trace:
  exporters: [file, stream]
  dir: ./traces
server:
  port: 8080
prompts_dir: ./prompts
skills_dir: ./skills
workdir: .
max_turns: 12
script_timeout: 60s
```

常用环境变量：`LLM_PROVIDER`、`LLM_BASE_URL`、`LLM_MODEL`、`LLM_API_KEY`、`SERVER_PORT`、`SKILLS_DIR`、`PROMPTS_DIR`、`WORKDIR`。

### 接本地模型示例

```yaml
# ollama
llm: { provider: ollama, base_url: http://localhost:11434/v1, model: qwen2.5 }
# llama.cpp（多数本地模型不支持原生工具调用，已默认启用模拟层）
llm: { provider: llamacpp, base_url: http://localhost:8080/v1, model: local }
```

## 使用

### CLI

```bash
./skill-runner -list                       # 列出 skill
./skill-runner "跟我打个招呼，我叫小明"      # 执行任务
./skill-runner -v "..."                    # 打印运行透视(span)日志
echo "跟我打个招呼" | ./skill-runner         # 管道输入
```

### Web

```bash
./skill-server                 # 默认 http://localhost:8080
```

浏览器打开后：左侧输入任务并执行，右侧「运行透视」实时展示调用树（LLM 生成、工具调用、token、耗时、输入/输出），「历史」可回看既往运行。

## 内置工具

模型可调用：`list_skills`、`load_skill`、`run_script`、`read_file`、`write_file`。

## 开发前端

```bash
cd web
npm install
npm run dev      # 开发模式，代理 /api 到 :8080
npm run build    # 产物输出到 web/dist（被 Go 内嵌）
```

修改前端后需 `npm run build` 再 `go build` 才会生效。

## 路线图

- 已完成：CLI + Web、配置系统、统一提示词、多 provider + 工具模拟层、运行透视（内置）。
- 后续：Langfuse 导出器、Wails 桌面端、向量检索选择器、更强沙箱、流式 token 输出。
