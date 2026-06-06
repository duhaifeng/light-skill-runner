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

// RegisterBuiltins 注册第一版的内置工具集。
func RegisterBuiltins(r *Registry, reg *registry.Registry, exec *executor.Executor, workDir string) {
	r.Register(listSkillsTool(reg))
	r.Register(loadSkillTool(reg))
	r.Register(runScriptTool(exec))
	r.Register(readFileTool(workDir))
	r.Register(writeFileTool(workDir))
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

func readFileTool(workDir string) Tool {
	return Tool{
		Spec: llm.ToolSpec{
			Name:        "read_file",
			Description: "读取工作目录内某个文件的文本内容。",
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
			full, err := resolveInDir(workDir, strArg(args, "path"))
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

func writeFileTool(workDir string) Tool {
	return Tool{
		Spec: llm.ToolSpec{
			Name:        "write_file",
			Description: "向工作目录内的文件写入文本内容（覆盖写入，必要时创建目录）。",
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
			full, err := resolveInDir(workDir, strArg(args, "path"))
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

// resolveInDir 将路径限制在 dir 之内，防止越界。
func resolveInDir(dir, path string) (string, error) {
	absDir, _ := filepath.Abs(dir)
	full := path
	if !filepath.IsAbs(full) {
		full = filepath.Join(absDir, path)
	}
	full = filepath.Clean(full)
	rel, err := filepath.Rel(absDir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("路径越界，禁止访问: %s", path)
	}
	return full, nil
}
