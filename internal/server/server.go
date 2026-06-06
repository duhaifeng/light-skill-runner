// Package server 提供 Web/API 服务（REST + SSE），供浏览器与桌面端复用。
package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/duhaifeng/light-skill-runner/internal/engine"
	"github.com/duhaifeng/light-skill-runner/internal/store"
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
	mux.HandleFunc("GET /api/settings/skills", s.handleSkillSettings)
	mux.HandleFunc("PUT /api/settings/skills/{name}", s.handleUpdateSkillSetting)
	mux.HandleFunc("GET /api/models", s.handleModels)
	mux.HandleFunc("POST /api/models", s.handleCreateModel)
	mux.HandleFunc("PUT /api/models/{id}", s.handleUpdateModel)
	mux.HandleFunc("POST /api/models/{id}/default", s.handleSetDefaultModel)
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

func (s *Server) handleSkillSettings(w http.ResponseWriter, r *http.Request) {
	items, err := s.eng.SkillSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleUpdateSkillSetting(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var body struct {
		Enabled   bool   `json:"enabled"`
		Tags      string `json:"tags"`
		SortOrder int    `json:"sort_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体无效"})
		return
	}
	if err := s.eng.UpdateSkillSetting(name, body.Enabled, body.Tags, body.SortOrder); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	models, err := s.eng.Models()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, models)
}

func (s *Server) handleCreateModel(w http.ResponseWriter, r *http.Request) {
	var m store.ModelConfig
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体无效"})
		return
	}
	if err := s.eng.CreateModel(m); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (s *Server) handleUpdateModel(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	var m store.ModelConfig
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体无效"})
		return
	}
	if err := s.eng.UpdateModel(id, m); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSetDefaultModel(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"))
	if !ok {
		return
	}
	if err := s.eng.SetDefaultModel(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleRun 同步执行，返回最终结果与 traceId（不流式）。
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Prompt string `json:"prompt"`
		Skill  string `json:"skill"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 prompt"})
		return
	}
	prompt := forceSkillPrompt(body.Prompt, body.Skill)
	res, err := s.eng.Run(r.Context(), prompt)
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
	prompt = forceSkillPrompt(prompt, r.URL.Query().Get("skill"))
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "不支持流式", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
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

func forceSkillPrompt(prompt, skill string) string {
	skill = strings.TrimSpace(skill)
	if skill == "" {
		return prompt
	}
	return "本次任务必须使用名为 `" + skill + "` 的 skill 执行。\n" +
		"请先调用 load_skill 读取该 skill 的完整说明，再严格按照该 skill 的说明和用户输入完成任务。\n" +
		"不要自行改用其他 skill。\n\n" +
		"用户输入：\n" + prompt
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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func parseID(w http.ResponseWriter, raw string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效 id"})
		return 0, false
	}
	return id, true
}
