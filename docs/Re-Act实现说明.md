# ReAct 实现说明

本文说明 `light-skill-runner` 中 ReAct 机制的落地方式，对应代码主要在 `internal/runner/runner.go`。

## 1. 什么是 ReAct

ReAct = **Re**asoning + **Act**ing（Yao et al., 2022）。让大模型把**推理**与**行动**交替进行，形成循环：

```text
Thought（思考/决策） → Action（调用工具） → Observation（观察工具结果） → 再 Thought …
```

直到模型认为信息足够，给出最终答案。相比纯推理（易幻觉、无法获取外部信息）和纯行动（不会规划），ReAct 能边想边做、用真实结果纠正推理，并且每一步可追溯。

## 2. 总体落地结构

引擎用一个 `for` 循环承载 ReAct 过程，用不断增长的 `messages` 切片作为 ReAct 轨迹（trajectory，即短期记忆）。每轮的 Thought / Action / Observation 都追加进 `messages`，使模型每次都能基于完整历史决策。

```text
初始 messages = [system(规则+工具说明), user(任务)]
        │
        ▼
┌──────────── for turn := 1..MaxTurns ────────────┐
│  Thought/决策:  chat(messages[, tools]) → resp   │
│        │                                         │
│   要调用工具?──否──▶ 返回最终答案（循环结束）       │
│        │是                                       │
│   Action:       callTool(name, args) → result    │
│   Observation:  把 result 追加回 messages         │
│        └────────────── 进入下一轮 ───────────────┘
└──────────────────────────────────────────────────┘
（达到 MaxTurns 仍未收尾 → 熔断报错）
```

引擎据 provider 能力选择两条等价路径（见 `Run`）：

- `runNative`：现代模型用结构化 `tool_calls` 字段表达 Action（原生 function-calling）。
- `runEmulated`：本地/弱模型用约定的 JSON 文本表达 Action，更接近 ReAct 原论文的纯文本协议。

路径选择在 `internal/engine/engine.go` 的 `buildSystemPrompt` 中根据 `client.Capabilities().SupportsTools` 决定；模拟模式会额外注入 `prompts/tool_emulation.tmpl` 协议说明。

## 3. 原生路径 `runNative`

ReAct 阶段与代码的对应：

| ReAct 阶段 | 代码 | 说明 |
|-----------|------|------|
| 每轮迭代 | `for turn := 1; turn <= r.MaxTurns` | 一次迭代 = 一个 Thought→Action→Observation 周期 |
| Thought + 决策 | `r.chat(ctx, turn, {Messages, Tools: specs})` | 把历史 + 工具清单发给模型；模型用「是否返回 tool_calls」表达下一步决策 |
| 终止判定 | `if len(resp.Message.ToolCalls) == 0` | 无工具调用 ⇒ 模型认为可直接回答，content 即最终答案 |
| 写回决策 | `messages = append(messages, resp.Message)` | 把含 tool_calls 的 assistant 消息写回（协议要求 tool 结果紧随其后） |
| Action | `r.callTool(ctx, parentID, tc.Name, tc.Arguments)` | 逐个执行模型请求的工具 |
| Observation | `append(..., {Role: RoleTool, ToolCallID: tc.ID, Content: result})` | 工具结果以 `role:tool` 回灌，用 `ToolCallID` 关联请求 |

**决策权完全在模型**：引擎不判断“任务做完没”，只看模型本轮**有没有发起工具调用**。`specs`（`r.Tools.Specs()`）定义了模型可选的 Action 空间。

## 4. 模拟路径 `runEmulated`

不下发 `Tools`，靠 `tool_emulation.tmpl` 约定模型每轮只输出两种 JSON 之一：

```json
{"tool_call": {"name": "工具名", "arguments": { ... }}}   // Action
{"final": "给用户的最终回答"}                               // 结束
```

处理流程：

1. **解析意图** `parseEmuReply` → `extractJSON`：用括号配平扫描，从模型输出里**宽松**抠出第一个 JSON 对象，容忍 ```json 代码块或多余文字。
2. **分支决策**：
   - `reply.Final != nil` ⇒ 结束，返回最终答案。
   - `reply.ToolCall != nil` ⇒ 执行工具，结果以 `role:user` 回灌（本地模型未必认 `tool` 角色，用 user 更稳），并显式提示“请据此继续输出 tool_call 或 final”。
3. 解析失败或两者皆空 ⇒ 容错：把模型原文当最终答案返回。

## 5. 如何利用模型判断“下一个动作”

| | 原生路径 | 模拟路径 |
|---|---|---|
| Action 的表达 | 结构化 `tool_calls` 字段 | 约定 JSON 文本 `{"tool_call":...}` |
| “继续/结束”的信号 | 有无 `tool_calls` | `tool_call` vs `final` |
| Observation 回灌角色 | `role:tool`（带 ToolCallID） | `role:user`（带提示语） |
| 解析方式 | API 保证结构 | `parseEmuReply` 宽松提取 |

两者本质相同：**引擎不替模型决定下一步，只识别模型表达出来的动作意图并执行、回灌**。

## 6. 何时结束：四类终止条件

| 终止条件 | 原生路径 | 模拟路径 | 含义 |
|---------|---------|---------|------|
| 模型主动收尾 | 无 `tool_calls`，返回 content | 输出 `{"final":...}` | 正常完成 |
| 模型未按协议 | —（API 保证） | 解析失败 / 无 tool_call·final ⇒ 原文当答案 | 容错兜底 |
| 达到步数上限 | `MaxTurns` 熔断 | `MaxTurns` 熔断 | 防止无限循环 / 失控烧 token |
| 调用出错 | `chat` 返回 err | `chat` 返回 err | LLM 请求失败 |

**正常结束的判定权在模型**；`MaxTurns`（默认 12，见 `Runner.New`）是唯一由引擎强制的安全边界。

## 7. 与运行透视（tracing）的关系

ReAct 的每一步都会被打点为 span（见 `chat` / `callTool`）：

- `chat`（Thought/决策）→ `generation` span，记录输入、输出、token 用量；
- `callTool`（Action）→ `tool-call` span，记录工具入参与执行结果。

因此 Web 透视界面里能直接看到完整的 Thought → Action → Observation 轨迹与每步耗时。

## 8. 执行示例（hello-world，原生路径）

```text
轮1  Thought/Action: 模型据 system 中的 skill 清单 → tool_calls=[load_skill("hello-world")]
     Observation:    append role:tool = SKILL.md 正文
轮2  Thought/Action: 读了指令 → tool_calls=[run_script("skills/hello-world/scripts/greet.py","小明")]
     Observation:    append role:tool = "你好，小明！..."
轮3  Thought:        模型判断信息已足够 → 无 tool_calls，返回最终问候语（循环结束）
```

## 9. 已知局限与可优化点

当前 **Thought（推理）是隐式的**：原生路径思考藏在模型内部不可见；模拟协议只有 `tool_call`/`final`，没有显式 thought 字段。

标准 ReAct 强调先写出 Thought 再 Action，对复杂任务与弱模型更稳。可将模拟协议扩展为：

```json
{"thought": "我需要先加载 hello-world 的指令", "tool_call": {"name": "load_skill", "arguments": {"name": "hello-world"}}}
```

引擎只执行 `tool_call` 部分，`thought` 既引导弱模型“先想再做”，又可作为 generation span 的可读输出，提升透视体验。
