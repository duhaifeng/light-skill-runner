package trace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// LogExporter 把每次运行过程写成可读日志，便于排查桌面端与 skill 执行问题。
type LogExporter struct {
	Dir     string
	Session string
}

// NewLogExporter 创建日志导出器。
func NewLogExporter(dir, session string) *LogExporter {
	if dir == "" {
		dir = "./logs"
	}
	if session == "" {
		session = time.Now().Format("20060102-150405")
	}
	_ = os.MkdirAll(dir, 0o755)
	return &LogExporter{Dir: dir, Session: session}
}

// Export 实现 Exporter。
func (e *LogExporter) Export(ev Event) {
	traceID := eventTraceID(ev)
	if traceID == "" {
		return
	}
	line := formatLogEvent(ev)
	if line == "" {
		return
	}
	_ = appendLog(filepath.Join(e.Dir, e.Session+".log"), line)
	_ = appendLog(filepath.Join(e.Dir, "runs.log"), line)
}

func appendLog(path, text string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(text)
	return err
}

func eventTraceID(ev Event) string {
	if ev.Trace != nil {
		return ev.Trace.ID
	}
	if ev.Span != nil {
		return ev.Span.TraceID
	}
	return ""
}

func formatLogEvent(ev Event) string {
	now := time.Now().Format(time.RFC3339Nano)
	var b strings.Builder
	switch ev.Kind {
	case EventTraceStart:
		if ev.Trace == nil {
			return ""
		}
		fmt.Fprintf(&b, "[%s] TRACE START trace=%s name=%s status=%s\n", now, ev.Trace.ID, ev.Trace.Name, ev.Trace.Status)
		writeBlock(&b, "INPUT", ev.Trace.Input)
	case EventSpanStart:
		if ev.Span == nil {
			return ""
		}
		fmt.Fprintf(&b, "[%s] SPAN START trace=%s span=%s type=%s name=%s parent=%s status=%s\n",
			now, ev.Span.TraceID, ev.Span.ID, ev.Span.Type, ev.Span.Name, ev.Span.ParentID, ev.Span.Status)
		writeBlock(&b, "INPUT", ev.Span.Input)
	case EventSpanEnd:
		if ev.Span == nil {
			return ""
		}
		fmt.Fprintf(&b, "[%s] SPAN END trace=%s span=%s type=%s name=%s status=%s duration=%s\n",
			now, ev.Span.TraceID, ev.Span.ID, ev.Span.Type, ev.Span.Name, ev.Span.Status, duration(ev.Span.StartTime, ev.Span.EndTime))
		if ev.Span.Usage != nil {
			fmt.Fprintf(&b, "TOKENS prompt=%d completion=%d total=%d\n", ev.Span.Usage.Prompt, ev.Span.Usage.Completion, ev.Span.Usage.Total)
		}
		if ev.Span.Error != "" {
			writeBlock(&b, "ERROR", ev.Span.Error)
		}
		writeBlock(&b, "OUTPUT", ev.Span.Output)
	case EventTraceEnd:
		if ev.Trace == nil {
			return ""
		}
		fmt.Fprintf(&b, "[%s] TRACE END trace=%s status=%s duration=%s\n",
			now, ev.Trace.ID, ev.Trace.Status, duration(ev.Trace.StartTime, ev.Trace.EndTime))
		if ev.Trace.Error != "" {
			writeBlock(&b, "ERROR", ev.Trace.Error)
		}
		writeBlock(&b, "OUTPUT", ev.Trace.Output)
	default:
		return ""
	}
	b.WriteString("\n")
	return b.String()
}

func writeBlock(b *strings.Builder, title, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(b, "%s:\n%s\n", title, value)
}

func duration(start, end time.Time) time.Duration {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	return end.Sub(start)
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
