import { useMemo, useState } from "react";
import type { Span, Trace } from "../types";

function duration(s: Span): string {
  if (!s.end_time) return "运行中…";
  const ms = new Date(s.end_time).getTime() - new Date(s.start_time).getTime();
  return ms >= 1000 ? `${(ms / 1000).toFixed(2)}s` : `${ms}ms`;
}

const typeLabel: Record<string, string> = {
  generation: "LLM",
  "tool-call": "工具",
  "skill-select": "选择",
};

function SpanNode({
  span,
  childrenOf,
}: {
  span: Span;
  childrenOf: (id: string) => Span[];
}) {
  const [open, setOpen] = useState(true);
  const kids = childrenOf(span.id);
  const statusClass =
    span.status === "ok" ? "ok" : span.status === "error" ? "err" : "running";
  return (
    <div className="span">
      <div className="span-head" onClick={() => setOpen(!open)}>
        <span className={`badge ${span.type}`}>
          {typeLabel[span.type] ?? span.type}
        </span>
        <span className="span-name">{span.name}</span>
        <span className={`dot ${statusClass}`} />
        <span className="span-dur">{duration(span)}</span>
        {span.usage && span.usage.total > 0 && (
          <span className="tokens">{span.usage.total} tok</span>
        )}
      </div>
      {open && (
        <div className="span-body">
          {span.input && (
            <div className="kv">
              <div className="k">输入</div>
              <pre>{span.input}</pre>
            </div>
          )}
          {span.output && (
            <div className="kv">
              <div className="k">输出</div>
              <pre>{span.output}</pre>
            </div>
          )}
          {span.error && (
            <div className="kv">
              <div className="k err">错误</div>
              <pre className="err">{span.error}</pre>
            </div>
          )}
          {kids.length > 0 && (
            <div className="children">
              {kids.map((c) => (
                <SpanNode key={c.id} span={c} childrenOf={childrenOf} />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export default function TraceView({ trace }: { trace: Trace | null }) {
  const { roots, childrenOf } = useMemo(() => {
    const spans = trace?.spans ?? [];
    const roots = spans.filter((s) => !s.parent_id);
    const childrenOf = (id: string) => spans.filter((s) => s.parent_id === id);
    return { roots, childrenOf };
  }, [trace]);

  if (!trace) {
    return <div className="empty">尚无运行。在左侧输入任务并执行。</div>;
  }

  return (
    <div className="trace">
      <div className="trace-meta">
        <span
          className={`dot ${
            trace.status === "ok"
              ? "ok"
              : trace.status === "error"
              ? "err"
              : "running"
          }`}
        />
        <span className="trace-id">trace {trace.id}</span>
      </div>
      <div className="span-list">
        {roots.map((s) => (
          <SpanNode key={s.id} span={s} childrenOf={childrenOf} />
        ))}
      </div>
      {trace.output && (
        <div className="final">
          <div className="k">最终回答</div>
          <pre>{trace.output}</pre>
        </div>
      )}
      {trace.error && (
        <div className="final err">
          <div className="k">错误</div>
          <pre>{trace.error}</pre>
        </div>
      )}
    </div>
  );
}
