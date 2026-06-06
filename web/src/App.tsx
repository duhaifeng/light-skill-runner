import { useCallback, useEffect, useRef, useState } from "react";
import {
  getModels,
  getRun,
  getRuns,
  getSkillSettings,
  getSkills,
  runStreamURL,
  saveModel,
  setDefaultModel,
  updateSkillSetting,
} from "./api";
import type {
  ModelConfig,
  RunSummary,
  Skill,
  SkillSetting,
  Span,
  Trace,
  TraceEvent,
} from "./types";
import TraceView from "./components/TraceView";

type Tab = "run" | "history" | "settings";

const emptyModel: ModelConfig = {
  name: "",
  provider: "socrates-gw",
  base_url: "https://socrates-llm-gw.jd.com/v1",
  model: "Qwen3.5-397B-A17B-FP8",
  api_key: "",
  force_tool_emulation: true,
  is_default: false,
};

export default function App() {
  const [skills, setSkills] = useState<Skill[]>([]);
  const [prompt, setPrompt] = useState("");
  const [selectedSkill, setSelectedSkill] = useState("");
  const [running, setRunning] = useState(false);
  const [runError, setRunError] = useState("");
  const [trace, setTrace] = useState<Trace | null>(null);
  const [tab, setTab] = useState<Tab>("run");
  const [runs, setRuns] = useState<RunSummary[]>([]);
  const [models, setModels] = useState<ModelConfig[]>([]);
  const [modelDraft, setModelDraft] = useState<ModelConfig>(emptyModel);
  const [skillSettings, setSkillSettings] = useState<SkillSetting[]>([]);
  const [settingsMsg, setSettingsMsg] = useState("");
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

  const refreshSettings = useCallback(() => {
    Promise.all([getModels(), getSkillSettings()])
      .then(([nextModels, nextSkills]) => {
        setModels(nextModels);
        setSkillSettings(nextSkills);
      })
      .catch(() => setSettingsMsg("加载设置失败"));
  }, []);

  useEffect(() => {
    if (tab === "settings") refreshSettings();
  }, [tab, refreshSettings]);

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
    setRunError("");
    setTrace(null);
    setTab("run");

    const es = new EventSource(runStreamURL(prompt, selectedSkill));
    esRef.current = es;
    es.onmessage = (e) => {
      try {
        const ev: TraceEvent = JSON.parse(e.data);
        applyEvent(ev);
        if (ev.kind === "trace_start") setRunError("");
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
      setRunError("连接运行流失败，请确认桌面端本地 API 服务已启动。");
    };
  }, [prompt, running, selectedSkill, applyEvent]);

  const openHistory = useCallback((id: string) => {
    getRun(id).then((t) => {
      setTrace(t);
      setTab("run");
    });
  }, []);

  const saveModelDraft = useCallback(() => {
    saveModel(modelDraft)
      .then(() => {
        setSettingsMsg("模型配置已保存");
        setModelDraft(emptyModel);
        refreshSettings();
      })
      .catch(() => setSettingsMsg("保存模型配置失败"));
  }, [modelDraft, refreshSettings]);

  const makeDefault = useCallback(
    (id?: number) => {
      if (!id) return;
      setDefaultModel(id)
        .then(() => {
          setSettingsMsg("默认模型已切换");
          refreshSettings();
        })
        .catch(() => setSettingsMsg("切换默认模型失败"));
    },
    [refreshSettings]
  );

  const saveSkill = useCallback(
    (skill: SkillSetting) => {
      updateSkillSetting(skill)
        .then(() => {
          setSettingsMsg("Skill 设置已保存");
          getSkills().then(setSkills).catch(() => setSkills([]));
          refreshSettings();
        })
        .catch(() => setSettingsMsg("保存 Skill 设置失败"));
    },
    [refreshSettings]
  );

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="brand">
          <h1>light-skill-runner</h1>
          <p>AI Skill 执行引擎</p>
        </div>

        <div className="composer">
          <select
            value={selectedSkill}
            onChange={(e) => setSelectedSkill(e.target.value)}
            title="选择后会强制本次运行使用该 skill"
          >
            <option value="">自动选择 Skill</option>
            {skills.map((s) => (
              <option key={s.name} value={s.name}>
                {s.name}
              </option>
            ))}
          </select>
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
          {runError && <div className="inline-error">{runError}</div>}
        </div>

        <div className="skills">
          <div className="section-title">可用 Skill ({skills.length})</div>
          {skills.map((s) => (
            <div
              key={s.name}
              className="skill-item"
              onClick={() => {
                setSelectedSkill(s.name);
                setPrompt((p) => p || `使用 ${s.name}`);
              }}
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
          <button
            className={tab === "settings" ? "active" : ""}
            onClick={() => setTab("settings")}
          >
            设置
          </button>
        </div>

        {tab === "run" ? (
          <TraceView trace={trace} />
        ) : tab === "history" ? (
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
        ) : (
          <div className="settings">
            <div className="settings-head">
              <div>
                <h2>模型配置</h2>
                <p>本地数据库维护模型，默认模型会用于下一次运行。</p>
              </div>
              {settingsMsg && <span className="settings-msg">{settingsMsg}</span>}
            </div>

            <div className="model-grid">
              {models.map((m) => (
                <div className="model-card" key={m.id}>
                  <div className="model-title">
                    <strong>{m.name}</strong>
                    {m.is_default && <span className="pill">默认</span>}
                  </div>
                  <div className="muted-line">{m.provider}</div>
                  <div className="muted-line">{m.model}</div>
                  <div className="muted-line">{m.base_url}</div>
                  <div className="model-actions">
                    <button onClick={() => setModelDraft(m)}>编辑</button>
                    <button disabled={m.is_default} onClick={() => makeDefault(m.id)}>
                      设为默认
                    </button>
                  </div>
                </div>
              ))}
            </div>

            <div className="settings-form">
              <input
                placeholder="名称"
                value={modelDraft.name}
                onChange={(e) =>
                  setModelDraft({ ...modelDraft, name: e.target.value })
                }
              />
              <input
                placeholder="provider"
                value={modelDraft.provider}
                onChange={(e) =>
                  setModelDraft({ ...modelDraft, provider: e.target.value })
                }
              />
              <input
                placeholder="base_url"
                value={modelDraft.base_url}
                onChange={(e) =>
                  setModelDraft({ ...modelDraft, base_url: e.target.value })
                }
              />
              <input
                placeholder="model"
                value={modelDraft.model}
                onChange={(e) =>
                  setModelDraft({ ...modelDraft, model: e.target.value })
                }
              />
              <input
                placeholder="api_key"
                value={modelDraft.api_key ?? ""}
                onChange={(e) =>
                  setModelDraft({ ...modelDraft, api_key: e.target.value })
                }
              />
              <label className="check-line">
                <input
                  type="checkbox"
                  checked={modelDraft.force_tool_emulation}
                  onChange={(e) =>
                    setModelDraft({
                      ...modelDraft,
                      force_tool_emulation: e.target.checked,
                    })
                  }
                />
                工具调用模拟
              </label>
              <label className="check-line">
                <input
                  type="checkbox"
                  checked={modelDraft.is_default}
                  onChange={(e) =>
                    setModelDraft({ ...modelDraft, is_default: e.target.checked })
                  }
                />
                保存为默认模型
              </label>
              <button onClick={saveModelDraft}>
                {modelDraft.id ? "保存模型" : "新增模型"}
              </button>
            </div>

            <div className="settings-head compact">
              <div>
                <h2>Skill 管理</h2>
                <p>Skill 内容仍保存在文件系统，这里维护启用状态、标签和排序。</p>
              </div>
            </div>
            <div className="skill-table">
              {skillSettings.map((s, idx) => (
                <div className="skill-row" key={s.name}>
                  <label className="check-line">
                    <input
                      type="checkbox"
                      checked={s.enabled}
                      onChange={(e) => {
                        const next = [...skillSettings];
                        next[idx] = { ...s, enabled: e.target.checked };
                        setSkillSettings(next);
                      }}
                    />
                    {s.name}
                  </label>
                  <input
                    placeholder="tags"
                    value={s.tags}
                    onChange={(e) => {
                      const next = [...skillSettings];
                      next[idx] = { ...s, tags: e.target.value };
                      setSkillSettings(next);
                    }}
                  />
                  <input
                    type="number"
                    value={s.sort_order}
                    onChange={(e) => {
                      const next = [...skillSettings];
                      next[idx] = { ...s, sort_order: Number(e.target.value) };
                      setSkillSettings(next);
                    }}
                  />
                  <button onClick={() => saveSkill(s)}>保存</button>
                  <span className="skill-path" title={s.path}>
                    {s.description}
                  </span>
                </div>
              ))}
              {skillSettings.length === 0 && <div className="empty">暂无 Skill。</div>}
            </div>
          </div>
        )}
      </main>
    </div>
  );
}
