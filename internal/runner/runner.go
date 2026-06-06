// Package runner 编排模型与工具之间的 ReAct 循环（Reasoning + Acting）。
//
// ReAct 的核心：模型在「推理(Thought) → 行动(Action) → 观察(Observation)」之间反复迭代，
// 每一轮都基于完整历史决定下一步，直到模型不再要求行动为止。本包用一个 for 循环承载该过程，
// 用不断增长的 messages 作为 ReAct 轨迹(trajectory)，并支持两种 Action 表达方式：
//   - runNative：现代模型用结构化的 tool_calls 字段表达 Action（原生 function-calling）；
//   - runEmulated：本地/弱模型用约定的 JSON 文本表达 Action（更接近 ReAct 原论文做法）。
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/duhaifeng/light-skill-runner/internal/llm"
	"github.com/duhaifeng/light-skill-runner/internal/tools"
	"github.com/duhaifeng/light-skill-runner/internal/trace"
)

// Runner 驱动一次完整的 Agentic Loop。
type Runner struct {
	Client       llm.Client
	Tools        *tools.Registry
	MaxTurns     int
	EmulateTools bool          // 为 true 时使用提示词模拟工具调用（本地模型）
	Tracer       *trace.Tracer // 可为 nil
}

// New 创建 Runner。
func New(client llm.Client, toolReg *tools.Registry, maxTurns int, emulate bool, tracer *trace.Tracer) *Runner {
	if maxTurns <= 0 {
		maxTurns = 12
	}
	return &Runner{Client: client, Tools: toolReg, MaxTurns: maxTurns, EmulateTools: emulate, Tracer: tracer}
}

// Run 执行一次任务，systemPrompt 已由上层组装完成（模拟模式下应已含协议说明）。
func (r *Runner) Run(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// messages 是整条 ReAct 轨迹（短期记忆）：
	// 初始仅含系统提示(规则/工具说明) + 用户任务；
	// 此后每轮的 Thought / Action / Observation 都会追加进来，
	// 使模型每次都能基于完整历史判断下一步动作。
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: systemPrompt},
		{Role: llm.RoleUser, Content: userPrompt},
	}
	if r.EmulateTools {
		return r.runEmulated(ctx, messages)
	}
	return r.runNative(ctx, messages)
}

// chat 执行 ReAct 的一次「Thought/决策」：调用 LLM，并记录一个 generation span 供透视。
func (r *Runner) chat(ctx context.Context, turn int, req llm.ChatRequest) (llm.ChatResponse, error) {
	var span *trace.Span
	if r.Tracer != nil {
		span = r.Tracer.StartSpan("", fmt.Sprintf("LLM 调用 (第%d轮)", turn), trace.SpanGeneration, lastContent(req.Messages))
	}
	resp, err := r.Client.Chat(ctx, req)
	if r.Tracer != nil {
		out := resp.Message.Content
		if len(resp.Message.ToolCalls) > 0 {
			out = summarizeToolCalls(resp.Message.ToolCalls)
		}
		r.Tracer.EndSpan(span, out, &trace.TokenUsage{
			Prompt:     resp.Usage.PromptTokens,
			Completion: resp.Usage.CompletionTokens,
			Total:      resp.Usage.TotalTokens,
		}, err)
	}
	return resp, err
}

// callTool 执行 ReAct 的一次「Action」：调用工具，并记录一个 tool-call span（挂在指定父 span 下）。
func (r *Runner) callTool(ctx context.Context, parentID, name, args string) string {
	var span *trace.Span
	if r.Tracer != nil {
		span = r.Tracer.StartSpan(parentID, name, trace.SpanToolCall, args)
	}
	result, err := r.Tools.Call(ctx, name, args)
	if err != nil {
		result = "错误: " + err.Error()
	}
	if r.Tracer != nil {
		r.Tracer.EndSpan(span, result, nil, err)
	}
	return result
}

// runNative 走原生 function-calling 路径：用结构化 tool_calls 表达 Action。
func (r *Runner) runNative(ctx context.Context, messages []llm.Message) (string, error) {
	// specs 即可用工具清单，随每次请求发给模型，作为它可选的 Action 空间。
	specs := r.Tools.Specs()

	// ===== ReAct 主循环：一次迭代 = 一个 Thought → Action → Observation 周期 =====
	// MaxTurns 是引擎强制的安全边界（熔断），防止模型陷入无限循环。
	for turn := 1; turn <= r.MaxTurns; turn++ {
		// --- Thought + 决策 ---
		// 把完整历史 messages + 工具清单交给模型；模型内部推理后，
		// 用「是否返回 tool_calls」来表达它对下一步动作的决策。
		resp, err := r.chat(ctx, turn, llm.ChatRequest{Messages: messages, Tools: specs})
		if err != nil {
			// [终止·出错] LLM 调用失败，直接结束。
			return "", fmt.Errorf("第 %d 轮调用失败: %w", turn, err)
		}

		// --- 终止判定 ---
		// [终止·正常] 模型未发起任何工具调用 => 它认为信息已足够，content 即最终答案。
		if len(resp.Message.ToolCalls) == 0 {
			return resp.Message.Content, nil
		}

		// 先把模型本轮的 assistant 消息（含 tool_calls）写回轨迹，
		// 否则下一轮模型看不到“自己请求了什么”，且 OpenAI 协议要求 tool 结果紧跟其后。
		messages = append(messages, resp.Message)

		// --- Action + Observation ---
		// 逐个执行模型请求的工具(Action)，并把结果以 role:tool 回灌(Observation)，
		// 通过 ToolCallID 与对应的请求一一关联；下一轮模型据此继续推理。
		var parentID string
		for _, tc := range resp.Message.ToolCalls {
			result := r.callTool(ctx, parentID, tc.Name, tc.Arguments) // Action
			messages = append(messages, llm.Message{ // Observation
				Role:       llm.RoleTool,
				ToolCallID: tc.ID,
				Content:    result,
			})
		}
	}
	// [终止·熔断] 达到最大轮数仍未收尾。
	return "", fmt.Errorf("达到最大轮数 %d 仍未完成", r.MaxTurns)
}

// emuReply 是模拟模式下期望模型输出的 JSON 结构：
// 模型每轮只能二选一——发起一次工具调用(tool_call=Action)，或给出最终答案(final=结束)。
type emuReply struct {
	ToolCall *struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"tool_call"`
	Final *string `json:"final"`
}

// runEmulated 走提示词模拟路径：不下发 tools，靠约定的 JSON 文本表达 Action / 结束。
// 用于不支持原生 function-calling 的本地/弱模型，更接近 ReAct 原论文的纯文本协议。
func (r *Runner) runEmulated(ctx context.Context, messages []llm.Message) (string, error) {
	// ===== ReAct 主循环（模拟版）=====
	for turn := 1; turn <= r.MaxTurns; turn++ {
		// --- Thought + 决策 ---
		// 不发送 Tools；模型按 tool_emulation 协议输出一段 JSON 表达其决策。
		resp, err := r.chat(ctx, turn, llm.ChatRequest{Messages: messages})
		if err != nil {
			// [终止·出错]
			return "", fmt.Errorf("第 %d 轮调用失败: %w", turn, err)
		}
		content := resp.Message.Content
		messages = append(messages, llm.Message{Role: llm.RoleAssistant, Content: content})

		// 从模型输出中宽松解析意图（容忍代码块包裹与多余文字）。
		reply, ok := parseEmuReply(content)
		if !ok {
			// [终止·容错] 模型未遵守协议（解析不出 tool_call/final），把原文当最终答案。
			return content, nil
		}
		if reply.Final != nil {
			// [终止·正常] 模型主动用 final 收尾。
			return *reply.Final, nil
		}
		if reply.ToolCall == nil {
			// [终止·容错] 既非 tool_call 也非 final。
			return content, nil
		}

		// --- Action + Observation ---
		// 执行模型请求的工具(Action)，并把结果以 role:user 回灌(Observation)——
		// 本地模型未必识别 tool 角色，故用 user 消息更稳；并显式提示进入下一轮。
		args := string(reply.ToolCall.Arguments)
		result := r.callTool(ctx, "", reply.ToolCall.Name, args) // Action
		messages = append(messages, llm.Message{ // Observation
			Role:    llm.RoleUser,
			Content: fmt.Sprintf("工具 %s 的执行结果：\n%s\n\n请据此继续（输出 tool_call 或 final 的 JSON）。", reply.ToolCall.Name, result),
		})
	}
	// [终止·熔断]
	return "", fmt.Errorf("达到最大轮数 %d 仍未完成", r.MaxTurns)
}

// parseEmuReply 从模型输出中宽松提取 JSON 并解析为 emuReply。
func parseEmuReply(content string) (emuReply, bool) {
	raw := extractJSON(content)
	if raw == "" {
		return emuReply{}, false
	}
	var reply emuReply
	if err := json.Unmarshal([]byte(raw), &reply); err != nil {
		return emuReply{}, false
	}
	if reply.ToolCall == nil && reply.Final == nil {
		return emuReply{}, false
	}
	return reply, true
}

// extractJSON 提取首个平衡的 JSON 对象（容忍代码块包裹与额外文字）。
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case ch == '\\':
				esc = true
			case ch == '"':
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

func lastContent(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Content != "" {
			return messages[i].Content
		}
	}
	return ""
}

func summarizeToolCalls(calls []llm.ToolCall) string {
	var parts []string
	for _, c := range calls {
		parts = append(parts, fmt.Sprintf("%s(%s)", c.Name, c.Arguments))
	}
	return "工具调用: " + strings.Join(parts, ", ")
}
