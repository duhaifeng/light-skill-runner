// Package config 负责加载并合并配置（默认值 → YAML 文件 → 环境变量）。
package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 是引擎的全部可配置项。
type Config struct {
	LLM    LLMConfig    `yaml:"llm"`
	Trace  TraceConfig  `yaml:"trace"`
	Server ServerConfig `yaml:"server"`
	Tools  ToolsConfig  `yaml:"tools"`

	PromptsDir    string        `yaml:"prompts_dir"`
	SkillsDir     string        `yaml:"skills_dir"`
	WorkDir       string        `yaml:"workdir"`
	DatabasePath  string        `yaml:"database_path"`
	MaxTurns      int           `yaml:"max_turns"`
	ScriptTimeout time.Duration `yaml:"script_timeout"`
}

// LLMConfig 描述 LLM provider 配置。
type LLMConfig struct {
	Provider string `yaml:"provider"` // openai | ollama | llamacpp | ...
	BaseURL  string `yaml:"base_url"`
	Model    string `yaml:"model"`
	APIKey   string `yaml:"api_key"`
	// ForceToolEmulation 为 true 时，无论 provider 是否支持原生 function-calling，
	// 都走提示词模拟的工具调用路径（用于本地小模型）。
	ForceToolEmulation bool `yaml:"force_tool_emulation"`
}

// TraceConfig 描述运行透视的导出配置。
type TraceConfig struct {
	Exporters []string `yaml:"exporters"` // file | stream | langfuse
	Dir       string   `yaml:"dir"`
	LogDir    string   `yaml:"log_dir"`
}

// ServerConfig 描述 Web 服务配置。
type ServerConfig struct {
	Port int `yaml:"port"`
}

// ToolsConfig 描述内置工具的能力与安全配置。
type ToolsConfig struct {
	// AllowArbitraryPaths 为 true 时关闭"限制在工作目录内"的沙箱，
	// 允许 read_file/write_file/run_script/run_command 访问任意路径。
	AllowArbitraryPaths bool          `yaml:"allow_arbitrary_paths"`
	Command             CommandConfig `yaml:"command"`
}

// CommandConfig 描述 run_command 工具的配置。
type CommandConfig struct {
	Enabled bool `yaml:"enabled"` // 是否注册 run_command 工具
	// Whitelist 是允许执行的程序名（按 basename 不区分大小写匹配）。
	Whitelist []string      `yaml:"whitelist"`
	Timeout   time.Duration `yaml:"timeout"` // 单条命令超时
	// EmptyWhitelistDenies 为 true 时，空白名单表示"拒绝全部"（更安全）。
	EmptyWhitelistDenies bool `yaml:"empty_whitelist_denies"`
}

// Default 返回带合理默认值的配置。
func Default() Config {
	return Config{
		LLM: LLMConfig{
			Provider: "openai",
			BaseURL:  "https://api.openai.com/v1",
		},
		Trace: TraceConfig{
			Exporters: []string{"file", "log", "stream"},
			Dir:       "./traces",
			LogDir:    "./logs",
		},
		Server: ServerConfig{Port: 8080},
		Tools: ToolsConfig{
			AllowArbitraryPaths: false, // 安全默认：保持工作目录沙箱
			Command: CommandConfig{
				Enabled:              true,
				Whitelist:            []string{"cmd", "powershell", "pwsh", "where", "go", "python", "node", "git", "ls", "dir", "cat", "type"},
				Timeout:              30 * time.Second,
				EmptyWhitelistDenies: true,
			},
		},
		PromptsDir:    "./prompts",
		SkillsDir:     "./skills",
		WorkDir:       ".",
		DatabasePath:  "./data/light-skill-runner.db",
		MaxTurns:      12,
		ScriptTimeout: 60 * time.Second,
	}
}

// Load 读取配置：默认值 → 可选 YAML 文件 → 环境变量覆盖。
// path 为空或文件不存在时跳过文件这一步（不报错）。
func Load(path string) (Config, error) {
	cfg := Default()

	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return cfg, err
			}
		} else if !os.IsNotExist(err) {
			return cfg, err
		}
	}

	applyEnv(&cfg)
	return cfg, nil
}

// applyEnv 用环境变量覆盖配置。
func applyEnv(cfg *Config) {
	if v := os.Getenv("LLM_PROVIDER"); v != "" {
		cfg.LLM.Provider = v
	}
	if v := os.Getenv("LLM_BASE_URL"); v != "" {
		cfg.LLM.BaseURL = v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		cfg.LLM.Model = v
	}
	if v := os.Getenv("LLM_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("SKILLS_DIR"); v != "" {
		cfg.SkillsDir = v
	}
	if v := os.Getenv("PROMPTS_DIR"); v != "" {
		cfg.PromptsDir = v
	}
	if v := os.Getenv("WORKDIR"); v != "" {
		cfg.WorkDir = v
	}
	if v := os.Getenv("DATABASE_PATH"); v != "" {
		cfg.DatabasePath = v
	}
	if v := os.Getenv("SERVER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	if v := os.Getenv("TOOLS_ALLOW_ARBITRARY_PATHS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Tools.AllowArbitraryPaths = b
		}
	}
	if v := os.Getenv("TOOLS_COMMAND_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Tools.Command.Enabled = b
		}
	}
	if v := os.Getenv("TOOLS_COMMAND_WHITELIST"); v != "" {
		var list []string
		for _, item := range strings.Split(v, ",") {
			if s := strings.TrimSpace(item); s != "" {
				list = append(list, s)
			}
		}
		cfg.Tools.Command.Whitelist = list
	}
}
