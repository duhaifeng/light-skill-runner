// Command skill-server 启动 light-skill-runner 的 Web/API 服务。
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/duhaifeng/light-skill-runner/internal/config"
	"github.com/duhaifeng/light-skill-runner/internal/engine"
	"github.com/duhaifeng/light-skill-runner/internal/server"
	"github.com/duhaifeng/light-skill-runner/web"
)

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal("加载配置失败: %v", err)
	}
	eng, err := engine.New(cfg)
	if err != nil {
		fatal("初始化引擎失败: %v", err)
	}

	srv := server.New(eng, web.DistFS())
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	fmt.Printf("light-skill-runner 已启动: http://localhost%s\n", addr)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		fatal("服务异常退出: %v", err)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
