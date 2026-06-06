// Package engine 是引擎门面，统一 CLI / Web / 桌面三端的执行入口。
package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/duhaifeng/light-skill-runner/internal/config"
	"github.com/duhaifeng/light-skill-runner/internal/executor"
	"github.com/duhaifeng/light-skill-runner/internal/llm"
	"github.com/duhaifeng/light-skill-runner/internal/loader"
	"github.com/duhaifeng/light-skill-runner/internal/prompt"
	"github.com/duhaifeng/light-skill-runner/internal/registry"
	"github.com/duhaifeng/light-skill-runner/internal/runner"
	"github.com/duhaifeng/light-skill-runner/internal/tools"
	"github.com/duhaifeng/light-skill-runner/internal/trace"
)

// Engine 持有跨次运行复用的资源（配置、skill 注册表、提示词、LLM 客户端）。
type Engine struct {
	cfg     config.Config
	reg     *registry.Registry
	prompts *prompt.Manager
	client  llm.Client
}

// RunResult 是一次运行的结果。
type RunResult struct {
	TraceID string `json:"trace_id"`
	Output  string `json:"output"`
}

// New 根据配置构建引擎：加载 skill、初始化提示词与 LLM 客户端。
func New(cfg config.Config) (*Engine, error) {
	skills, err := loader.Load(cfg.SkillsDir)
	if err != nil {
		return nil, fmt.Errorf("加载 skill 失败: %w", err)
	}
	client, err := llm.New(cfg.LLM.Provider, llm.ProviderConfig{
		BaseURL:            cfg.LLM.BaseURL,
		Model:              cfg.LLM.Model,
		APIKey:             cfg.LLM.APIKey,
		ForceToolEmulation: cfg.LLM.ForceToolEmulation,
	})
	if err != nil {
		return nil, err
	}
	return &Engine{
		cfg:     cfg,
		reg:     registry.New(skills),
		prompts: prompt.New(cfg.PromptsDir),
		client:  client,
	}, nil
}

// Config 返回引擎配置。
func (e *Engine) Config() config.Config { return e.cfg }

// Skills 返回已加载的 skill 列表。
func (e *Engine) Skills() []loader.Skill { return e.reg.List() }

// Run 执行一次任务。extraExporters 可附加导出器（如 SSE 流）。
func (e *Engine) Run(ctx context.Context, userPrompt string, extraExporters ...trace.Exporter) (RunResult, error) {
	// 组装导出器：配置中的 file + 调用方附加的。
	var exporters []trace.Exporter
	for _, name := range e.cfg.Trace.Exporters {
		if name == "file" {
			exporters = append(exporters, trace.NewFileExporter(e.cfg.Trace.Dir))
		}
	}
	exporters = append(exporters, extraExporters...)

	tracer := trace.NewTracer("skill-run", userPrompt, exporters...)

	// 每次运行构建工具集（绑定 workdir / 超时）。
	exec := executor.New(e.cfg.WorkDir, e.cfg.ScriptTimeout)
	toolReg := tools.New()
	tools.RegisterBuiltins(toolReg, e.reg, exec, e.cfg.WorkDir)

	systemPrompt, emulate, err := e.buildSystemPrompt(toolReg)
	if err != nil {
		tracer.Finish("", err)
		return RunResult{TraceID: tracer.TraceID()}, err
	}

	r := runner.New(e.client, toolReg, e.cfg.MaxTurns, emulate, tracer)
	output, runErr := r.Run(ctx, systemPrompt, userPrompt)
	tracer.Finish(output, runErr)

	return RunResult{TraceID: tracer.TraceID(), Output: output}, runErr
}

// buildSystemPrompt 组装系统提示，并根据 provider 能力决定是否启用工具调用模拟。
func (e *Engine) buildSystemPrompt(toolReg *tools.Registry) (string, bool, error) {
	skills := make([]prompt.SkillInfo, 0, e.reg.Len())
	for _, s := range e.reg.List() {
		skills = append(skills, prompt.SkillInfo{Name: s.Name, Description: s.Description})
	}
	system, err := e.prompts.System(skills)
	if err != nil {
		return "", false, err
	}

	emulate := !e.client.Capabilities().SupportsTools
	if !emulate {
		return system, false, nil
	}

	toolInfos := make([]prompt.ToolInfo, 0)
	for _, spec := range toolReg.Specs() {
		schema, _ := json.Marshal(spec.Parameters)
		toolInfos = append(toolInfos, prompt.ToolInfo{
			Name:        spec.Name,
			Description: spec.Description,
			Schema:      string(schema),
		})
	}
	emu, err := e.prompts.ToolEmulation(toolInfos)
	if err != nil {
		return "", false, err
	}
	return system + "\n\n" + emu, true, nil
}
