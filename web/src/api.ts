import type { RunSummary, Skill, Trace } from "./types";

export async function getSkills(): Promise<Skill[]> {
  const r = await fetch("/api/skills");
  return (await r.json()) ?? [];
}

export async function getRuns(): Promise<RunSummary[]> {
  const r = await fetch("/api/runs");
  return (await r.json()) ?? [];
}

export async function getRun(id: string): Promise<Trace> {
  const r = await fetch(`/api/runs/${id}`);
  return await r.json();
}

export function runStreamURL(prompt: string): string {
  return `/api/run/stream?prompt=${encodeURIComponent(prompt)}`;
}
