import type { ModelConfig, RunSummary, Skill, SkillSetting, Trace } from "./types";

declare global {
  interface Window {
    __LSR_API_BASE__?: string;
  }
}

const API_BASE =
  window.__LSR_API_BASE__ ??
  (window.location.protocol === "http:" || window.location.protocol === "https:"
    ? ""
    : "http://127.0.0.1:8080");

function apiURL(path: string): string {
  return `${API_BASE}${path}`;
}

export async function getSkills(): Promise<Skill[]> {
  const r = await fetch(apiURL("/api/skills"));
  return (await r.json()) ?? [];
}

export async function getSkillSettings(): Promise<SkillSetting[]> {
  const r = await fetch(apiURL("/api/settings/skills"));
  return (await r.json()) ?? [];
}

export async function updateSkillSetting(skill: SkillSetting): Promise<void> {
  await fetch(apiURL(`/api/settings/skills/${encodeURIComponent(skill.name)}`), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      enabled: skill.enabled,
      tags: skill.tags,
      sort_order: skill.sort_order,
    }),
  });
}

export async function getModels(): Promise<ModelConfig[]> {
  const r = await fetch(apiURL("/api/models"));
  return (await r.json()) ?? [];
}

export async function saveModel(model: ModelConfig): Promise<void> {
  const id = model.id;
  await fetch(apiURL(id ? `/api/models/${id}` : "/api/models"), {
    method: id ? "PUT" : "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(model),
  });
}

export async function setDefaultModel(id: number): Promise<void> {
  await fetch(apiURL(`/api/models/${id}/default`), { method: "POST" });
}

export async function getRuns(): Promise<RunSummary[]> {
  const r = await fetch(apiURL("/api/runs"));
  return (await r.json()) ?? [];
}

export async function getRun(id: string): Promise<Trace> {
  const r = await fetch(apiURL(`/api/runs/${id}`));
  return await r.json();
}

export function runStreamURL(prompt: string, skill?: string): string {
  const params = new URLSearchParams({ prompt });
  if (skill) params.set("skill", skill);
  return apiURL(`/api/run/stream?${params.toString()}`);
}
