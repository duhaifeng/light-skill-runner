// Package engine 是引擎门面，统一 CLI / Web / 桌面三端的执行入口。
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/duhaifeng/light-skill-runner/internal/config"
	"github.com/duhaifeng/light-skill-runner/internal/executor"
	"github.com/duhaifeng/light-skill-runner/internal/llm"
	"github.com/duhaifeng/light-skill-runner/internal/loader"
	"github.com/duhaifeng/light-skill-runner/internal/prompt"
	"github.com/duhaifeng/light-skill-runner/internal/registry"
	"github.com/duhaifeng/light-skill-runner/internal/runner"
	"github.com/duhaifeng/light-skill-runner/internal/store"
	"github.com/duhaifeng/light-skill-runner/internal/tools"
	"github.com/duhaifeng/light-skill-runner/internal/trace"
)

// Engine 持有跨次运行复用的资源（配置、skill 注册表、提示词、LLM 客户端）。
type Engine struct {
	mu      sync.Mutex
	cfg     config.Config
	reg     *registry.Registry
	prompts *prompt.Manager
	client  llm.Client
	store   *store.Store
	logID   string
}

// RunResult 是一次运行的结果。
type RunResult struct {
	TraceID string `json:"trace_id"`
	Output  string `json:"output"`
}

// New 根据配置构建引擎：加载 skill、初始化提示词与 LLM 客户端。
func New(cfg config.Config) (*Engine, error) {
	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	if err := st.SeedModel(store.ModelConfig{
		Name:               cfg.LLM.Provider + " / " + cfg.LLM.Model,
		Provider:           cfg.LLM.Provider,
		BaseURL:            cfg.LLM.BaseURL,
		Model:              cfg.LLM.Model,
		APIKey:             cfg.LLM.APIKey,
		ForceToolEmulation: cfg.LLM.ForceToolEmulation,
	}); err != nil {
		return nil, fmt.Errorf("初始化模型配置失败: %w", err)
	}
	if m, ok, err := st.DefaultModel(); err != nil {
		return nil, fmt.Errorf("读取默认模型配置失败: %w", err)
	} else if ok {
		applyModel(&cfg, m)
	}

	skills, err := loadEnabledSkills(cfg.SkillsDir, st)
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
		store:   st,
		logID:   time.Now().Format("20060102-150405"),
	}, nil
}

// Config 返回引擎配置。
func (e *Engine) Config() config.Config {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cfg
}

// Skills 返回已加载的 skill 列表。
func (e *Engine) Skills() []loader.Skill {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.reg.List()
}

// SkillSettings 返回 UI 可维护的 skill 元信息。
func (e *Engine) SkillSettings() ([]store.SkillSetting, error) {
	return e.store.ListSkillSettings()
}

// UpdateSkillSetting 更新 skill 设置并重新加载内存注册表。
func (e *Engine) UpdateSkillSetting(name string, enabled bool, tags string, sortOrder int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.store.UpdateSkillSetting(name, enabled, tags, sortOrder); err != nil {
		return err
	}
	skills, err := loadEnabledSkills(e.cfg.SkillsDir, e.store)
	if err != nil {
		return err
	}
	e.reg = registry.New(skills)
	return nil
}

// Models 返回本地维护的模型配置。
func (e *Engine) Models() ([]store.ModelConfig, error) {
	return e.store.ListModels()
}

// CreateModel 新增模型配置。
func (e *Engine) CreateModel(m store.ModelConfig) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.store.CreateModel(m); err != nil {
		return err
	}
	if m.IsDefault {
		return e.reloadDefaultModelLocked()
	}
	return nil
}

// UpdateModel 更新模型配置。
func (e *Engine) UpdateModel(id int64, m store.ModelConfig) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.store.UpdateModel(id, m); err != nil {
		return err
	}
	if m.IsDefault {
		return e.reloadDefaultModelLocked()
	}
	return nil
}

// SetDefaultModel 切换默认模型配置，并重建 LLM client。
func (e *Engine) SetDefaultModel(id int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.store.SetDefaultModel(id); err != nil {
		return err
	}
	return e.reloadDefaultModelLocked()
}

// Run 执行一次任务。extraExporters 可附加导出器（如 SSE 流）。
func (e *Engine) Run(ctx context.Context, userPrompt string, extraExporters ...trace.Exporter) (RunResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 组装导出器：配置中的 file + 调用方附加的。
	var exporters []trace.Exporter
	for _, name := range e.cfg.Trace.Exporters {
		if name == "file" {
			exporters = append(exporters, trace.NewFileExporter(e.cfg.Trace.Dir))
		}
		if name == "log" {
			exporters = append(exporters, trace.NewLogExporter(e.cfg.Trace.LogDir, e.logID))
		}
	}
	exporters = append(exporters, extraExporters...)

	tracer := trace.NewTracer("skill-run", userPrompt, exporters...)

	// 每次运行构建工具集（绑定 workdir / 超时 / 安全开关）。
	exec := executor.New(e.cfg.WorkDir, e.cfg.ScriptTimeout, e.cfg.Tools.AllowArbitraryPaths, e.cfg.Tools.Command.Timeout)
	toolReg := tools.New()
	tools.RegisterBuiltins(toolReg, e.reg, exec, e.cfg.WorkDir, tools.Options{
		AllowArbitraryPaths: e.cfg.Tools.AllowArbitraryPaths,
		Command: tools.CommandOptions{
			Enabled:              e.cfg.Tools.Command.Enabled,
			Whitelist:            e.cfg.Tools.Command.Whitelist,
			EmptyWhitelistDenies: e.cfg.Tools.Command.EmptyWhitelistDenies,
		},
	})

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

func (e *Engine) reloadDefaultModelLocked() error {
	m, ok, err := e.store.DefaultModel()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("没有默认模型配置")
	}
	applyModel(&e.cfg, m)
	client, err := llm.New(e.cfg.LLM.Provider, llm.ProviderConfig{
		BaseURL:            e.cfg.LLM.BaseURL,
		Model:              e.cfg.LLM.Model,
		APIKey:             e.cfg.LLM.APIKey,
		ForceToolEmulation: e.cfg.LLM.ForceToolEmulation,
	})
	if err != nil {
		return err
	}
	e.client = client
	return nil
}

func loadEnabledSkills(skillsDir string, st *store.Store) ([]loader.Skill, error) {
	skills, err := loader.Load(skillsDir)
	if err != nil {
		return nil, err
	}
	sources := make([]store.SkillSource, 0, len(skills))
	for _, sk := range skills {
		sources = append(sources, store.SkillSource{
			Name:        sk.Name,
			Description: sk.Description,
			Path:        sk.Path,
		})
	}
	if err := st.SyncSkills(sources); err != nil {
		return nil, err
	}
	enabled, err := st.EnabledSkillNames()
	if err != nil {
		return nil, err
	}
	out := make([]loader.Skill, 0, len(skills))
	for _, sk := range skills {
		if enabled[sk.Name] {
			out = append(out, sk)
		}
	}
	return out, nil
}

// osInfo 返回当前运行环境描述，帮助模型选择正确的命令语法。
func osInfo() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows (命令行用 cmd 或 powershell；列目录如 run_command cmd [\"/c\",\"dir\",\"C:\\\\\"])"
	case "darwin":
		return "macOS (命令行用 sh/bash；列目录如 run_command ls [\"-la\",\"/\"])"
	default:
		return runtime.GOOS + " (类 Unix，命令行用 sh/bash)"
	}
}

func applyModel(cfg *config.Config, m store.ModelConfig) {
	cfg.LLM.Provider = m.Provider
	cfg.LLM.BaseURL = m.BaseURL
	cfg.LLM.Model = m.Model
	cfg.LLM.APIKey = m.APIKey
	cfg.LLM.ForceToolEmulation = m.ForceToolEmulation
}

// buildSystemPrompt 组装系统提示，并根据 provider 能力决定是否启用工具调用模拟。
func (e *Engine) buildSystemPrompt(toolReg *tools.Registry) (string, bool, error) {
	skills := make([]prompt.SkillInfo, 0, e.reg.Len())
	for _, s := range e.reg.List() {
		skills = append(skills, prompt.SkillInfo{Name: s.Name, Description: s.Description})
	}
	system, err := e.prompts.System(skills, osInfo())
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
