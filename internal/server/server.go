// Package server 提供 Web/API 服务（REST + SSE），供浏览器与桌面端复用。
package server

import (
	"encoding/json"
	"io/fs"
	"net/http"

	"github.com/duhaifeng/light-skill-runner/internal/engine"
	"github.com/duhaifeng/light-skill-runner/internal/trace"
)

// Server 包装引擎并暴露 HTTP 接口。
type Server struct {
	eng    *engine.Engine
	static fs.FS // 前端静态资源，可为 nil
}

// New 创建 Server。static 为内嵌的前端资源（可为 nil）。
func New(eng *engine.Engine, static fs.FS) *Server {
	return &Server{eng: eng, static: static}
}

// Handler 返回路由处理器。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/skills", s.handleSkills)
	mux.HandleFunc("GET /api/run/stream", s.handleRunStream)
	mux.HandleFunc("POST /api/runs", s.handleRun)
	mux.HandleFunc("GET /api/runs", s.handleRunsList)
	mux.HandleFunc("GET /api/runs/{id}", s.handleRunDetail)

	if s.static != nil {
		mux.Handle("/", http.FileServer(http.FS(s.static)))
	}
	return mux
}

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	type item struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	var out []item
	for _, sk := range s.eng.Skills() {
		out = append(out, item{Name: sk.Name, Description: sk.Description})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRun 同步执行，返回最终结果与 traceId（不流式）。
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 prompt"})
		return
	}
	res, err := s.eng.Run(r.Context(), body.Prompt)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"trace_id": res.TraceID, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleRunStream 通过 SSE 实时推送 trace 事件（EventSource，GET）。
func (s *Server) handleRunStream(w http.ResponseWriter, r *http.Request) {
	prompt := r.URL.Query().Get("prompt")
	if prompt == "" {
		http.Error(w, "缺少 prompt", http.StatusBadRequest)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "不支持流式", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	stream := trace.NewStreamExporter(256)
	done := make(chan struct{})
	go func() {
		_, _ = s.eng.Run(r.Context(), prompt, stream)
		close(done)
	}()

	enc := json.NewEncoder(w)
	for {
		select {
		case ev := <-stream.Events():
			w.Write([]byte("data: "))
			_ = enc.Encode(ev) // Encode 自带换行
			w.Write([]byte("\n"))
			flusher.Flush()
			if ev.Kind == trace.EventTraceEnd {
				return
			}
		case <-done:
			// 兜底：运行结束后清空缓冲事件。
			for {
				select {
				case ev := <-stream.Events():
					w.Write([]byte("data: "))
					_ = enc.Encode(ev)
					w.Write([]byte("\n"))
					flusher.Flush()
				default:
					return
				}
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleRunsList(w http.ResponseWriter, r *http.Request) {
	list, err := trace.List(s.eng.Config().Trace.Dir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := trace.Read(s.eng.Config().Trace.Dir, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "找不到该 trace"})
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
