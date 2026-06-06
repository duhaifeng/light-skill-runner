// Command skill-desktop 启动 light-skill-runner 的桌面端。
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/duhaifeng/light-skill-runner/internal/config"
	"github.com/duhaifeng/light-skill-runner/internal/engine"
	"github.com/duhaifeng/light-skill-runner/internal/server"
	"github.com/duhaifeng/light-skill-runner/web"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
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
	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Server.Port)
	httpSrv, err := startLocalServer(addr, srv.Handler())
	if err != nil {
		fatal("启动本地 API 服务失败: %v", err)
	}
	defer httpSrv.Close()

	if err := wails.Run(&options.App{
		Title:  "light-skill-runner",
		Width:  1280,
		Height: 820,
		AssetServer: &assetserver.Options{
			Assets:  web.DistFS(),
			Handler: desktopConfig(addr),
		},
		BackgroundColour: &options.RGBA{R: 15, G: 17, B: 21, A: 1},
	}); err != nil {
		fatal("桌面端启动失败: %v", err)
	}
}

func desktopConfig(addr string) http.Handler {
	apiBase := "http://" + addr
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/desktop-config.js" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		fmt.Fprintf(w, "window.__LSR_API_BASE__ = %q;\n", apiBase)
	})
}

func startLocalServer(addr string, handler http.Handler) (*http.Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: allowLocalDesktop(handler)}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "本地 API 服务异常退出: %v\n", err)
		}
	}()
	return srv, nil
}

func allowLocalDesktop(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
