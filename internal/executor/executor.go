// Package executor 在受控环境中执行 skill 携带的脚本。
package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// Executor 负责执行脚本/命令，可选地限定工作目录并施加超时。
type Executor struct {
	WorkDir string        // 脚本/命令的默认工作目录
	Timeout time.Duration // 脚本执行超时
	// AllowArbitraryPaths 为 true 时关闭工作目录沙箱，允许访问任意路径。
	AllowArbitraryPaths bool
	CommandTimeout      time.Duration // run_command 的超时
}

// New 创建一个执行器。
func New(workDir string, timeout time.Duration, allowArbitraryPaths bool, commandTimeout time.Duration) *Executor {
	abs, _ := filepath.Abs(workDir)
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if commandTimeout <= 0 {
		commandTimeout = 30 * time.Second
	}
	return &Executor{
		WorkDir:             abs,
		Timeout:             timeout,
		AllowArbitraryPaths: allowArbitraryPaths,
		CommandTimeout:      commandTimeout,
	}
}

// RunScript 根据扩展名选择解释器执行脚本，返回合并后的输出。
// path 必须位于 WorkDir 之内。
func (e *Executor) RunScript(ctx context.Context, path string, args []string) (string, error) {
	full, err := e.resolve(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(full); err != nil {
		return "", fmt.Errorf("脚本不存在: %s", path)
	}

	name, cmdArgs := interpreter(full)
	cmdArgs = append(cmdArgs, args...)

	ctx, cancel := context.WithTimeout(ctx, e.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, cmdArgs...)
	cmd.Dir = e.WorkDir
	cmd.Env = append(os.Environ(),
		"PYTHONIOENCODING=utf-8",
		"PYTHONUTF8=1",
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err = cmd.Run()
	out := decodeOutput(buf.Bytes())
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("脚本执行超时 (%s)", e.Timeout)
	}
	if err != nil {
		return out, fmt.Errorf("脚本退出异常: %w", err)
	}
	return out, nil
}

// RunCommand 直接执行一个程序（不经过 shell），返回退出码与合并输出。
// 白名单校验由调用方（工具层）完成；cwd 为空时使用 WorkDir。
func (e *Executor) RunCommand(ctx context.Context, program string, args []string, cwd string) (string, error) {
	timeout := e.CommandTimeout
	if timeout <= 0 {
		timeout = e.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dir := cwd
	if dir == "" {
		dir = e.WorkDir
	} else if !e.AllowArbitraryPaths {
		// 沙箱开启时，cwd 仍受工作目录限制。
		resolved, err := e.resolve(cwd)
		if err != nil {
			return "", err
		}
		dir = resolved
	}

	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"PYTHONIOENCODING=utf-8",
		"PYTHONUTF8=1",
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	out := decodeOutput(buf.Bytes())
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("命令执行超时 (%s)", timeout)
	}
	// 非零退出不视为硬错误：把退出码和 stderr 交给模型判断。
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Sprintf("exit_code=%d\n%s", exitErr.ExitCode(), out), nil
	}
	if err != nil {
		return out, fmt.Errorf("命令启动失败: %w", err)
	}
	return fmt.Sprintf("exit_code=0\n%s", out), nil
}

// resolve 将相对/绝对路径解析为绝对路径；沙箱开启时限制在 WorkDir 内防止越界。
func (e *Executor) resolve(path string) (string, error) {
	full := path
	if !filepath.IsAbs(full) {
		full = filepath.Join(e.WorkDir, path)
	}
	full = filepath.Clean(full)
	if e.AllowArbitraryPaths {
		return full, nil
	}
	rel, err := filepath.Rel(e.WorkDir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("路径越界，禁止访问: %s", path)
	}
	return full, nil
}

// interpreter 根据扩展名返回解释器及前置参数。
func interpreter(path string) (string, []string) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".py":
		return "python", []string{path}
	case ".js":
		return "node", []string{path}
	case ".sh":
		return "bash", []string{path}
	case ".ps1":
		return "powershell", []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", path}
	default:
		if runtime.GOOS == "windows" {
			return path, nil
		}
		return path, nil
	}
}

func decodeOutput(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}
	out, err := simplifiedchinese.GB18030.NewDecoder().Bytes(data)
	if err == nil {
		return string(out)
	}
	return string(data)
}
