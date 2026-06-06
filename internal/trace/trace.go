// Package trace 提供借鉴 Langfuse 的运行透视：一次运行=Trace，内部嵌套 Span。
package trace

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// SpanType 区分 span 的种类。
type SpanType string

const (
	SpanGeneration  SpanType = "generation"   // 一次 LLM 调用
	SpanToolCall    SpanType = "tool-call"     // 一次工具调用
	SpanSkillSelect SpanType = "skill-select"  // skill 选择
)

// TokenUsage 记录 token 用量。
type TokenUsage struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

// Span 是 trace 中的一个嵌套节点。
type Span struct {
	ID        string         `json:"id"`
	ParentID  string         `json:"parent_id,omitempty"`
	TraceID   string         `json:"trace_id"`
	Name      string         `json:"name"`
	Type      SpanType       `json:"type"`
	StartTime time.Time      `json:"start_time"`
	EndTime   time.Time      `json:"end_time,omitempty"`
	Input     string         `json:"input,omitempty"`
	Output    string         `json:"output,omitempty"`
	Status    string         `json:"status"` // running | ok | error
	Error     string         `json:"error,omitempty"`
	Usage     *TokenUsage    `json:"usage,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Trace 表示一次完整运行。
type Trace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time,omitempty"`
	Input     string    `json:"input,omitempty"`
	Output    string    `json:"output,omitempty"`
	Status    string    `json:"status"` // running | ok | error
	Error     string    `json:"error,omitempty"`
	Spans     []*Span   `json:"spans"`
}

// EventKind 表示导出事件类型。
type EventKind string

const (
	EventTraceStart EventKind = "trace_start"
	EventSpanStart  EventKind = "span_start"
	EventSpanEnd    EventKind = "span_end"
	EventTraceEnd   EventKind = "trace_end"
)

// Event 是推送给导出器的一条事件。
type Event struct {
	Kind  EventKind `json:"kind"`
	Trace *Trace    `json:"trace,omitempty"`
	Span  *Span     `json:"span,omitempty"`
}

// Exporter 消费 trace 事件（文件落盘、SSE 推送、Langfuse 等）。
type Exporter interface {
	Export(Event)
}

// Tracer 管理单次运行的 trace 与 span。并发安全。
type Tracer struct {
	mu        sync.Mutex
	trace     *Trace
	exporters []Exporter
}

// NewTracer 开始一次 trace，并广播 trace_start 事件。
func NewTracer(name, input string, exporters ...Exporter) *Tracer {
	t := &Tracer{
		trace: &Trace{
			ID:        newID(),
			Name:      name,
			StartTime: time.Now(),
			Input:     input,
			Status:    "running",
			Spans:     []*Span{},
		},
		exporters: exporters,
	}
	t.emit(Event{Kind: EventTraceStart, Trace: t.snapshotTrace()})
	return t
}

// TraceID 返回当前 trace id。
func (t *Tracer) TraceID() string { return t.trace.ID }

// StartSpan 开启一个 span。
func (t *Tracer) StartSpan(parentID, name string, typ SpanType, input string) *Span {
	t.mu.Lock()
	s := &Span{
		ID:        newID(),
		ParentID:  parentID,
		TraceID:   t.trace.ID,
		Name:      name,
		Type:      typ,
		StartTime: time.Now(),
		Status:    "running",
		Input:     input,
	}
	t.trace.Spans = append(t.trace.Spans, s)
	t.mu.Unlock()
	t.emit(Event{Kind: EventSpanStart, Span: snapshotSpan(s)})
	return s
}

// EndSpan 结束一个 span。
func (t *Tracer) EndSpan(s *Span, output string, usage *TokenUsage, err error) {
	t.mu.Lock()
	s.EndTime = time.Now()
	s.Output = output
	s.Usage = usage
	if err != nil {
		s.Status = "error"
		s.Error = err.Error()
	} else {
		s.Status = "ok"
	}
	t.mu.Unlock()
	t.emit(Event{Kind: EventSpanEnd, Span: snapshotSpan(s)})
}

// Finish 结束整个 trace。
func (t *Tracer) Finish(output string, err error) {
	t.mu.Lock()
	t.trace.EndTime = time.Now()
	t.trace.Output = output
	if err != nil {
		t.trace.Status = "error"
		t.trace.Error = err.Error()
	} else {
		t.trace.Status = "ok"
	}
	t.mu.Unlock()
	t.emit(Event{Kind: EventTraceEnd, Trace: t.snapshotTrace()})
}

// Snapshot 返回当前 trace 的拷贝。
func (t *Tracer) Snapshot() *Trace { return t.snapshotTrace() }

func (t *Tracer) emit(ev Event) {
	for _, e := range t.exporters {
		e.Export(ev)
	}
}

func (t *Tracer) snapshotTrace() *Trace {
	t.mu.Lock()
	defer t.mu.Unlock()
	cp := *t.trace
	cp.Spans = make([]*Span, len(t.trace.Spans))
	for i, s := range t.trace.Spans {
		cp.Spans[i] = snapshotSpan(s)
	}
	return &cp
}

func snapshotSpan(s *Span) *Span {
	cp := *s
	return &cp
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
