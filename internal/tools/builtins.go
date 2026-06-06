package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/duhaifeng/light-skill-runner/internal/executor"
	"github.com/duhaifeng/light-skill-runner/internal/llm"
	"github.com/duhaifeng/light-skill-runner/internal/registry"
)

// Options 描述内置工具的能力与安全开关。
type Options struct {
	// AllowArbitraryPaths 为 true 时，read_file/write_file 不再限制在工作目录内。
	AllowArbitraryPaths bool
	Command             CommandOptions
}

// CommandOptions 描述 run_command 工具的配置。
type CommandOptions struct {
	Enabled              bool
	Whitelist            []string
	EmptyWhitelistDenies bool
}

// RegisterBuiltins 注册内置工具集。
func RegisterBuiltins(r *Registry, reg *registry.Registry, exec *executor.Executor, workDir string, opts Options) {
	r.Register(listSkillsTool(reg))
	r.Register(loadSkillTool(reg))
	r.Register(runScriptTool(exec))
	r.Register(readFileTool(workDir, opts.AllowArbitraryPaths))
	r.Register(writeFileTool(workDir, opts.AllowArbitraryPaths))
	if opts.Command.Enabled {
		r.Register(runCommandTool(exec, opts.Command))
	}
}

// runCommandTool 执行系统命令（直接 exec，不经过 shell）。
// 需要 shell 内置命令/管道时，由模型显式调用 cmd/powershell/sh。
func runCommandTool(exec *executor.Executor, opts CommandOptions) Tool {
	return Tool{
		Spec: llm.ToolSpec{
			Name:        "run_command",
			Description: "执行一个系统命令（直接运行程序，不经过 shell）。需要 shell 内置命令(如 Windows 的 dir)或管道时，请调用 cmd/powershell/sh 并通过 args 传入子命令。返回 exit_code 与合并输出。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "要执行的程序名，如 cmd / powershell / go / python / git",
					},
					"args": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "命令参数列表，如 [\"/c\", \"dir\", \"C:\\\\\"]",
					},
					"cwd": map[string]any{
						"type":        "string",
						"description": "可选，命令的工作目录（绝对路径）",
					},
				},
				"required": []string{"command"},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			command := strArg(args, "command")
			if command == "" {
				return "", fmt.Errorf("command 不能为空")
			}
			if !commandAllowed(command, opts) {
				return "", fmt.Errorf("命令 %q 不在白名单内，已拒绝执行", command)
			}
			var cmdArgs []string
			if raw, ok := args["args"].([]any); ok {
				for _, v := range raw {
					if s, ok := v.(string); ok {
						cmdArgs = append(cmdArgs, s)
					}
				}
			}
			return exec.RunCommand(ctx, command, cmdArgs, strArg(args, "cwd"))
		},
	}
}

// commandAllowed 按 basename 不区分大小写匹配白名单。空白名单依据配置决定放行/拒绝。
func commandAllowed(command string, opts CommandOptions) bool {
	if len(opts.Whitelist) == 0 {
		return !opts.EmptyWhitelistDenies
	}
	name := strings.ToLower(filepath.Base(strings.TrimSpace(command)))
	name = strings.TrimSuffix(name, ".exe")
	for _, w := range opts.Whitelist {
		allowed := strings.ToLower(strings.TrimSpace(w))
		allowed = strings.TrimSuffix(allowed, ".exe")
		if name == allowed {
			return true
		}
	}
	return false
}

func listSkillsTool(reg *registry.Registry) Tool {
	return Tool{
		Spec: llm.ToolSpec{
			Name:        "list_skills",
			Description: "列出当前可用的全部 skill 及其简介。",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			var b strings.Builder
			for _, s := range reg.List() {
				fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
			}
			if b.Len() == 0 {
				return "（没有可用的 skill）", nil
			}
			return b.String(), nil
		},
	}
}

func loadSkillTool(reg *registry.Registry) Tool {
	return Tool{
		Spec: llm.ToolSpec{
			Name:        "load_skill",
			Description: "读取指定 skill 的完整 SKILL.md 正文指令。在使用某个 skill 前应先调用它。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "skill 名称",
					},
				},
				"required": []string{"name"},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			name := strArg(args, "name")
			s, ok := reg.Get(name)
			if !ok {
				return "", fmt.Errorf("找不到名为 %q 的 skill", name)
			}
			return fmt.Sprintf("# Skill: %s\n目录: %s\n\n%s", s.Name, s.Dir, s.Body), nil
		},
	}
}

func runScriptTool(exec *executor.Executor) Tool {
	return Tool{
		Spec: llm.ToolSpec{
			Name:        "run_script",
			Description: "在工作目录内执行一个脚本（支持 .py/.js/.sh/.ps1），返回其标准输出与错误输出。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "脚本路径（相对工作目录或其内的绝对路径）",
					},
					"args": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "传给脚本的参数列表",
					},
				},
				"required": []string{"path"},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			path := strArg(args, "path")
			var scriptArgs []string
			if raw, ok := args["args"].([]any); ok {
				for _, v := range raw {
					if s, ok := v.(string); ok {
						scriptArgs = append(scriptArgs, s)
					}
				}
			}
			out, err := exec.RunScript(ctx, path, scriptArgs)
			if err != nil {
				return out, err
			}
			if strings.TrimSpace(out) == "" {
				return "（脚本无输出，执行成功）", nil
			}
			return out, nil
		},
	}
}

func readFileTool(workDir string, allowArbitrary bool) Tool {
	return Tool{
		Spec: llm.ToolSpec{
			Name:        "read_file",
			Description: "读取某个文件的文本内容。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "文件路径（相对工作目录或其内的绝对路径）",
					},
				},
				"required": []string{"path"},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			full, err := resolveInDir(workDir, strArg(args, "path"), allowArbitrary)
			if err != nil {
				return "", err
			}
			data, err := os.ReadFile(full)
			if err != nil {
				return "", fmt.Errorf("读取文件失败: %w", err)
			}
			return string(data), nil
		},
	}
}

func writeFileTool(workDir string, allowArbitrary bool) Tool {
	return Tool{
		Spec: llm.ToolSpec{
			Name:        "write_file",
			Description: "向文件写入文本内容（覆盖写入，必要时创建目录）。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "文件路径（相对工作目录或其内的绝对路径）",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "要写入的文本内容",
					},
				},
				"required": []string{"path", "content"},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			full, err := resolveInDir(workDir, strArg(args, "path"), allowArbitrary)
			if err != nil {
				return "", err
			}
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(full, []byte(strArg(args, "content")), 0o644); err != nil {
				return "", fmt.Errorf("写入文件失败: %w", err)
			}
			return fmt.Sprintf("已写入 %s", full), nil
		},
	}
}

// resolveInDir 解析路径；allowArbitrary 为 false 时限制在 dir 之内防止越界。
func resolveInDir(dir, path string, allowArbitrary bool) (string, error) {
	absDir, _ := filepath.Abs(dir)
	full := path
	if !filepath.IsAbs(full) {
		full = filepath.Join(absDir, path)
	}
	full = filepath.Clean(full)
	if allowArbitrary {
		return full, nil
	}
	rel, err := filepath.Rel(absDir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("路径越界，禁止访问: %s", path)
	}
	return full, nil
}
