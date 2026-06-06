// Package config 负责加载并合并配置（默认值 → YAML 文件 → 环境变量）。
package config

import (
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 是引擎的全部可配置项。
type Config struct {
	LLM    LLMConfig    `yaml:"llm"`
	Trace  TraceConfig  `yaml:"trace"`
	Server ServerConfig `yaml:"server"`

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
		Server:        ServerConfig{Port: 8080},
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
}
