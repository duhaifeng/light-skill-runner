// Package prompt 统一管理提示词：内嵌默认模板 + 磁盘覆盖 + 模板渲染。
package prompt

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/duhaifeng/light-skill-runner/prompts"
)

// 模板名常量（对应 prompts/*.tmpl）。
const (
	TmplSystem        = "system"
	TmplToolEmulation = "tool_emulation"
)

// SkillInfo 用于渲染系统提示中的 skill 列表。
type SkillInfo struct {
	Name        string
	Description string
}

// ToolInfo 用于渲染工具模拟提示中的工具清单。
type ToolInfo struct {
	Name        string
	Description string
	Schema      string
}

// Manager 负责按名加载并渲染模板。
type Manager struct {
	diskDir string // 磁盘覆盖目录，留空则只用内嵌
}

// New 创建提示词管理器，diskDir 为可选的磁盘覆盖目录。
func New(diskDir string) *Manager {
	return &Manager{diskDir: diskDir}
}

// System 渲染主系统提示。
func (m *Manager) System(skills []SkillInfo) (string, error) {
	return m.render(TmplSystem, map[string]any{"Skills": skills})
}

// ToolEmulation 渲染本地模型工具调用模拟提示。
func (m *Manager) ToolEmulation(tools []ToolInfo) (string, error) {
	return m.render(TmplToolEmulation, map[string]any{"Tools": tools})
}

// render 加载模板（磁盘优先，回退内嵌）并用 data 渲染。
func (m *Manager) render(name string, data any) (string, error) {
	raw, err := m.load(name)
	if err != nil {
		return "", err
	}
	tmpl, err := template.New(name).Parse(raw)
	if err != nil {
		return "", fmt.Errorf("解析模板 %s 失败: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染模板 %s 失败: %w", name, err)
	}
	return buf.String(), nil
}

// load 读取模板原文：磁盘 diskDir/<name>.tmpl 优先，否则用内嵌。
func (m *Manager) load(name string) (string, error) {
	file := name + ".tmpl"
	if m.diskDir != "" {
		p := filepath.Join(m.diskDir, file)
		if data, err := os.ReadFile(p); err == nil {
			return string(data), nil
		}
	}
	data, err := prompts.FS.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("找不到模板 %s（磁盘与内嵌均无）", file)
	}
	return string(data), nil
}
