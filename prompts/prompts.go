// Package prompts 内嵌默认提示词模板，作为编译期单一来源。
// 运行期可通过配置的 prompts 目录在磁盘上覆盖同名模板（免重新编译）。
package prompts

import "embed"

//go:embed *.tmpl
var FS embed.FS
