export interface Skill {
  name: string;
  description: string;
}

export interface TokenUsage {
  prompt: number;
  completion: number;
  total: number;
}

export type SpanType = "generation" | "tool-call" | "skill-select";

export interface Span {
  id: string;
  parent_id?: string;
  trace_id: string;
  name: string;
  type: SpanType;
  start_time: string;
  end_time?: string;
  input?: string;
  output?: string;
  status: string;
  error?: string;
  usage?: TokenUsage;
}

export interface Trace {
  id: string;
  name: string;
  start_time: string;
  end_time?: string;
  input?: string;
  output?: string;
  status: string;
  error?: string;
  spans: Span[];
}

export type EventKind =
  | "trace_start"
  | "span_start"
  | "span_end"
  | "trace_end";

export interface TraceEvent {
  kind: EventKind;
  trace?: Trace;
  span?: Span;
}

export interface RunSummary {
  id: string;
  name: string;
  status: string;
  input: string;
  start_time: string;
}
