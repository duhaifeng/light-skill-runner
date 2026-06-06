import { useCallback, useEffect, useRef, useState } from "react";
import { getRun, getRuns, getSkills, runStreamURL } from "./api";
import type { RunSummary, Skill, Span, Trace, TraceEvent } from "./types";
import TraceView from "./components/TraceView";

type Tab = "run" | "history";

export default function App() {
  const [skills, setSkills] = useState<Skill[]>([]);
  const [prompt, setPrompt] = useState("");
  const [running, setRunning] = useState(false);
  const [trace, setTrace] = useState<Trace | null>(null);
  const [tab, setTab] = useState<Tab>("run");
  const [runs, setRuns] = useState<RunSummary[]>([]);
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    getSkills().then(setSkills).catch(() => setSkills([]));
  }, []);

  const refreshRuns = useCallback(() => {
    getRuns().then(setRuns).catch(() => setRuns([]));
  }, []);

  useEffect(() => {
    if (tab === "history") refreshRuns();
  }, [tab, refreshRuns]);

  const applyEvent = useCallback((ev: TraceEvent) => {
    setTrace((prev) => {
      if (ev.kind === "trace_start" && ev.trace) {
        return { ...ev.trace, spans: ev.trace.spans ?? [] };
      }
      if (!prev) return prev;
      if (ev.kind === "trace_end" && ev.trace) {
        return { ...prev, ...ev.trace, spans: ev.trace.spans ?? prev.spans };
      }
      if ((ev.kind === "span_start" || ev.kind === "span_end") && ev.span) {
        const span = ev.span as Span;
        const idx = prev.spans.findIndex((s) => s.id === span.id);
        const spans = [...prev.spans];
        if (idx >= 0) spans[idx] = span;
        else spans.push(span);
        return { ...prev, spans };
      }
      return prev;
    });
  }, []);

  const run = useCallback(() => {
    if (!prompt.trim() || running) return;
    setRunning(true);
    setTrace(null);
    setTab("run");

    const es = new EventSource(runStreamURL(prompt));
    esRef.current = es;
    es.onmessage = (e) => {
      try {
        const ev: TraceEvent = JSON.parse(e.data);
        applyEvent(ev);
        if (ev.kind === "trace_end") {
          es.close();
          esRef.current = null;
          setRunning(false);
        }
      } catch {
        /* ignore malformed frame */
      }
    };
    es.onerror = () => {
      es.close();
      esRef.current = null;
      setRunning(false);
    };
  }, [prompt, running, applyEvent]);

  const openHistory = useCallback((id: string) => {
    getRun(id).then((t) => {
      setTrace(t);
      setTab("run");
    });
  }, []);

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="brand">
          <h1>light-skill-runner</h1>
          <p>AI Skill 执行引擎</p>
        </div>

        <div className="composer">
          <textarea
            placeholder="描述你的任务，例如：跟我打个招呼，我叫小明"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) run();
            }}
          />
          <button onClick={run} disabled={running || !prompt.trim()}>
            {running ? "执行中…" : "执行 (Ctrl/Cmd+Enter)"}
          </button>
        </div>

        <div className="skills">
          <div className="section-title">可用 Skill ({skills.length})</div>
          {skills.map((s) => (
            <div
              key={s.name}
              className="skill-item"
              onClick={() => setPrompt((p) => p || `使用 ${s.name}`)}
              title={s.description}
            >
              <div className="skill-name">{s.name}</div>
              <div className="skill-desc">{s.description}</div>
            </div>
          ))}
          {skills.length === 0 && <div className="muted">未发现 skill</div>}
        </div>
      </aside>

      <main className="main">
        <div className="tabs">
          <button
            className={tab === "run" ? "active" : ""}
            onClick={() => setTab("run")}
          >
            运行透视
          </button>
          <button
            className={tab === "history" ? "active" : ""}
            onClick={() => setTab("history")}
          >
            历史
          </button>
        </div>

        {tab === "run" ? (
          <TraceView trace={trace} />
        ) : (
          <div className="history">
            {runs.length === 0 && <div className="empty">暂无历史运行。</div>}
            {runs.map((r) => (
              <div
                key={r.id}
                className="run-item"
                onClick={() => openHistory(r.id)}
              >
                <span
                  className={`dot ${
                    r.status === "ok"
                      ? "ok"
                      : r.status === "error"
                      ? "err"
                      : "running"
                  }`}
                />
                <span className="run-input">{r.input || "(无输入)"}</span>
                <span className="run-time">{r.start_time}</span>
              </div>
            ))}
          </div>
        )}
      </main>
    </div>
  );
}
