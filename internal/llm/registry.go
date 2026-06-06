package llm

import (
	"fmt"
	"sort"
)

// ProviderConfig 是创建 Client 所需的最小配置（与上层 config 解耦）。
type ProviderConfig struct {
	BaseURL            string
	Model              string
	APIKey             string
	ForceToolEmulation bool
}

// Factory 根据配置创建一个 Client。
type Factory func(ProviderConfig) (Client, error)

var providers = map[string]Factory{}

// Register 注册一个 provider 工厂。新增 provider 只需新增文件并在此注册。
func Register(name string, f Factory) {
	providers[name] = f
}

// New 按名称创建 Client。
func New(name string, cfg ProviderConfig) (Client, error) {
	f, ok := providers[name]
	if !ok {
		return nil, fmt.Errorf("未知 LLM provider: %q（可用: %v）", name, Providers())
	}
	return f(cfg)
}

// Providers 返回已注册的 provider 名称（排序后）。
func Providers() []string {
	names := make([]string, 0, len(providers))
	for n := range providers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
