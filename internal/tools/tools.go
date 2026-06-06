// Package tools 定义暴露给模型的工具及其注册表。
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/duhaifeng/light-skill-runner/internal/llm"
)

// Handler 执行一次工具调用，args 为已解析的参数。
type Handler func(ctx context.Context, args map[string]any) (string, error)

// Tool 是一个工具：规格 + 处理函数。
type Tool struct {
	Spec    llm.ToolSpec
	Handler Handler
}

// Registry 是工具注册表。
type Registry struct {
	order []string
	tools map[string]Tool
}

// New 创建空的工具注册表。
func New() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register 注册一个工具。
func (r *Registry) Register(t Tool) {
	if _, exists := r.tools[t.Spec.Name]; !exists {
		r.order = append(r.order, t.Spec.Name)
	}
	r.tools[t.Spec.Name] = t
}

// Specs 返回所有工具的规格，用于注入 ChatRequest。
func (r *Registry) Specs() []llm.ToolSpec {
	out := make([]llm.ToolSpec, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.tools[name].Spec)
	}
	return out
}

// Call 根据名称与原始 JSON 参数执行工具。
func (r *Registry) Call(ctx context.Context, name, argsJSON string) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("未知工具: %s", name)
	}
	args := map[string]any{}
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("工具 %s 参数解析失败: %w", name, err)
		}
	}
	return t.Handler(ctx, args)
}

// strArg 从参数中读取字符串。
func strArg(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
