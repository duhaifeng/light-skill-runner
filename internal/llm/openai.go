package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIClient 是一个 OpenAI 兼容的 provider 实现。
// 通过设置不同的 BaseURL，可对接 OpenAI / DeepSeek / Ollama / llama.cpp / vLLM 等。
type OpenAIClient struct {
	BaseURL            string
	APIKey             string
	Model              string
	ChatTemplateKwargs map[string]any
	Temperature        *float64 // 默认采样温度；nil 表示不下发
	MaxTokens          int      // 最大生成 token；0 表示不下发
	HTTP               *http.Client
	caps               Capabilities
}

// NewOpenAI 创建一个 OpenAI 兼容客户端。
// 默认 temperature=0：Agent 场景追求确定性，能显著降低工具调用 JSON 漂移。
func NewOpenAI(baseURL, apiKey, model string) *OpenAIClient {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	zero := 0.0
	return &OpenAIClient{
		BaseURL:     baseURL,
		APIKey:      apiKey,
		Model:       model,
		Temperature: &zero,
		HTTP:        &http.Client{Timeout: 120 * time.Second},
		caps:        Capabilities{SupportsTools: true, SupportsStreaming: false},
	}
}

// Capabilities 实现 Client 接口。
func (c *OpenAIClient) Capabilities() Capabilities { return c.caps }

// newOpenAICompatible 是注册到 provider 注册表的工厂构造器。
// supportsToolsDefault 给出该 provider 默认是否支持原生 function-calling，
// 当 cfg.ForceToolEmulation 为 true 时强制关闭（走模拟路径）。
func newOpenAICompatible(defaultBaseURL string, supportsToolsDefault bool) Factory {
	return func(cfg ProviderConfig) (Client, error) {
		base := cfg.BaseURL
		if base == "" {
			base = defaultBaseURL
		}
		c := NewOpenAI(base, cfg.APIKey, cfg.Model)
		c.caps.SupportsTools = supportsToolsDefault && !cfg.ForceToolEmulation
		return c, nil
	}
}

func init() {
	// 三者均走 OpenAI 兼容协议，差异在默认地址与默认能力。
	Register("openai", newOpenAICompatible("https://api.openai.com/v1", true))
	Register("ollama", newOpenAICompatible("http://localhost:11434/v1", true))
	Register("llamacpp", newOpenAICompatible("http://localhost:8080/v1", false))
	Register("socrates-gw", newSocratesGW)
}

// ---- 线缆数据结构（OpenAI Chat Completions 格式）----

type wireToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type wireResponseFormat struct {
	Type string `json:"type"`
}

type wireRequest struct {
	Model              string              `json:"model"`
	Messages           []wireMessage       `json:"messages"`
	Tools              []wireTool          `json:"tools,omitempty"`
	Temperature        *float64            `json:"temperature,omitempty"`
	MaxTokens          int                 `json:"max_tokens,omitempty"`
	ResponseFormat     *wireResponseFormat `json:"response_format,omitempty"`
	ChatTemplateKwargs map[string]any      `json:"chat_template_kwargs,omitempty"`
}

func newSocratesGW(cfg ProviderConfig) (Client, error) {
	base := cfg.BaseURL
	if base == "" {
		base = "https://socrates-llm-gw.jd.com/v1"
	}
	model := cfg.Model
	if model == "" {
		model = "Qwen3.5-397B-A17B-FP8"
	}
	c := NewOpenAI(base, cfg.APIKey, model)
	// 网关走 OpenAI 兼容协议，支持原生 tools；由 force_tool_emulation 开关控制是否启用。
	c.caps.SupportsTools = !cfg.ForceToolEmulation
	c.ChatTemplateKwargs = map[string]any{"enable_thinking": false}
	return c, nil
}

type wireResponse struct {
	Choices []struct {
		Message      wireMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Chat 实现 Client 接口。
func (c *OpenAIClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	wreq := wireRequest{
		Model:              c.Model,
		Temperature:        c.Temperature,
		MaxTokens:          c.MaxTokens,
		ChatTemplateKwargs: c.ChatTemplateKwargs,
	}
	if req.JSONMode {
		wreq.ResponseFormat = &wireResponseFormat{Type: "json_object"}
	}
	for _, m := range req.Messages {
		wm := wireMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			var w wireToolCall
			w.ID = tc.ID
			w.Type = "function"
			w.Function.Name = tc.Name
			w.Function.Arguments = tc.Arguments
			wm.ToolCalls = append(wm.ToolCalls, w)
		}
		wreq.Messages = append(wreq.Messages, wm)
	}
	for _, t := range req.Tools {
		var w wireTool
		w.Type = "function"
		w.Function.Name = t.Name
		w.Function.Description = t.Description
		w.Function.Parameters = t.Parameters
		wreq.Tools = append(wreq.Tools, w)
	}

	body, err := json.Marshal(wreq)
	if err != nil {
		return ChatResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	httpReq.Header.Set("Accept", "application/json")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("调用 LLM 失败: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("LLM 返回 %d: %s", resp.StatusCode, string(raw))
	}

	var wresp wireResponse
	if err := json.Unmarshal(raw, &wresp); err != nil {
		return ChatResponse{}, fmt.Errorf("解析 LLM 响应失败: %w; 原始: %s", err, string(raw))
	}
	if wresp.Error != nil {
		return ChatResponse{}, fmt.Errorf("LLM 错误: %s", wresp.Error.Message)
	}
	if len(wresp.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("LLM 未返回任何 choice")
	}

	choice := wresp.Choices[0]
	out := ChatResponse{
		FinishReason: choice.FinishReason,
		Usage: Usage{
			PromptTokens:     wresp.Usage.PromptTokens,
			CompletionTokens: wresp.Usage.CompletionTokens,
			TotalTokens:      wresp.Usage.TotalTokens,
		},
	}
	out.Message.Role = RoleAssistant
	out.Message.Content = choice.Message.Content
	for _, tc := range choice.Message.ToolCalls {
		out.Message.ToolCalls = append(out.Message.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return out, nil
}
