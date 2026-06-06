package trace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ConsoleExporter 把 span 事件打印到 stderr，用于 CLI 的 -v 模式。
type ConsoleExporter struct{}

// Export 实现 Exporter。
func (ConsoleExporter) Export(ev Event) {
	switch ev.Kind {
	case EventSpanStart:
		fmt.Fprintf(os.Stderr, "[%s] ▶ %s\n", ev.Span.Type, ev.Span.Name)
	case EventSpanEnd:
		out := ev.Span.Output
		if len(out) > 160 {
			out = out[:160] + "..."
		}
		fmt.Fprintf(os.Stderr, "[%s] ✓ %s -> %s\n", ev.Span.Type, ev.Span.Name, out)
	}
}

// FileExporter 在 trace 结束时把完整 trace 写入 <dir>/<traceID>.json。
type FileExporter struct {
	Dir string
}

// NewFileExporter 创建文件导出器并确保目录存在。
func NewFileExporter(dir string) *FileExporter {
	_ = os.MkdirAll(dir, 0o755)
	return &FileExporter{Dir: dir}
}

// Export 实现 Exporter。
func (e *FileExporter) Export(ev Event) {
	if ev.Kind != EventTraceEnd || ev.Trace == nil {
		return
	}
	data, err := json.MarshalIndent(ev.Trace, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(e.Dir, ev.Trace.ID+".json"), data, 0o644)
}

// StreamExporter 把事件非阻塞地转发到一个 channel，用于 SSE 实时推送。
type StreamExporter struct {
	ch chan Event
}

// NewStreamExporter 创建流式导出器。
func NewStreamExporter(buffer int) *StreamExporter {
	if buffer <= 0 {
		buffer = 64
	}
	return &StreamExporter{ch: make(chan Event, buffer)}
}

// Events 返回只读事件通道。
func (e *StreamExporter) Events() <-chan Event { return e.ch }

// Export 实现 Exporter（非阻塞，满了则丢弃以免拖慢运行）。
func (e *StreamExporter) Export(ev Event) {
	select {
	case e.ch <- ev:
	default:
	}
}

// Close 关闭事件通道。
func (e *StreamExporter) Close() { close(e.ch) }
