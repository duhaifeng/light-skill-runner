// Command skill-runner 是 light-skill-runner 的命令行入口。
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/duhaifeng/light-skill-runner/internal/config"
	"github.com/duhaifeng/light-skill-runner/internal/engine"
	"github.com/duhaifeng/light-skill-runner/internal/trace"

	"flag"
)

func main() {
	var (
		configPath = flag.String("config", "config.yaml", "配置文件路径")
		listOnly   = flag.Bool("list", false, "仅列出已发现的 skill 后退出")
		verbose    = flag.Bool("v", false, "打印运行透视（span）日志")
	)
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal("加载配置失败: %v", err)
	}

	eng, err := engine.New(cfg)
	if err != nil {
		fatal("初始化引擎失败: %v", err)
	}

	if *listOnly {
		skills := eng.Skills()
		if len(skills) == 0 {
			fmt.Println("（没有发现任何 skill）")
			return
		}
		fmt.Printf("发现 %d 个 skill：\n", len(skills))
		for _, s := range skills {
			fmt.Printf("- %s: %s\n", s.Name, s.Description)
		}
		return
	}

	userPrompt := strings.TrimSpace(strings.Join(flag.Args(), " "))
	if userPrompt == "" {
		userPrompt = readStdin()
	}
	if userPrompt == "" {
		fatal("请提供任务描述，例如: skill-runner \"帮我...\"")
	}
	if cfg.LLM.Model == "" {
		fatal("请在 config.yaml 或环境变量中设置 LLM 模型（LLM_MODEL）")
	}

	var extra []trace.Exporter
	if *verbose {
		extra = append(extra, trace.ConsoleExporter{})
	}

	res, err := eng.Run(context.Background(), userPrompt, extra...)
	if err != nil {
		fatal("执行失败 (trace=%s): %v", res.TraceID, err)
	}
	fmt.Println(res.Output)
}

func readStdin() string {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return ""
	}
	data, _ := io.ReadAll(os.Stdin)
	return strings.TrimSpace(string(data))
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
