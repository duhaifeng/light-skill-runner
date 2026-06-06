// Package llm 定义 provider 无关的 LLM 调用接口与数据类型。
package llm

import "context"

// Role 表示消息角色。
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall 表示模型发起的一次工具调用。
type ToolCall struct {
	ID        string // 调用 id，回传 tool 结果时需要
	Name      string // 工具名
	Arguments string // 原始 JSON 字符串形式的参数
}

// Message 是一条对话消息。
type Message struct {
	Role       Role
	Content    string
	ToolCalls  []ToolCall // 仅 assistant 消息可能携带
	ToolCallID string     // 仅 tool 消息使用，对应某次 ToolCall.ID
}

// ToolSpec 描述一个暴露给模型的工具。
type ToolSpec struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema
}

// ChatRequest 是一次对话请求。
type ChatRequest struct {
	Messages []Message
	Tools    []ToolSpec
}

// Usage 是一次调用的 token 用量。
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// ChatResponse 是一次对话响应。
type ChatResponse struct {
	Message      Message
	FinishReason string
	Usage        Usage
}

// Capabilities 描述某个 provider 的能力。
type Capabilities struct {
	SupportsTools     bool // 是否支持原生 function-calling
	SupportsStreaming bool // 是否支持流式输出
}

// Client 是所有 LLM provider 实现的统一接口。
type Client interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
	Capabilities() Capabilities
}
