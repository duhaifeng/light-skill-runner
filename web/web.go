// Package web 内嵌前端构建产物（web/dist），供 skill-server 以单二进制提供。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// DistFS 返回以 dist 为根的前端静态文件系统。
func DistFS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
