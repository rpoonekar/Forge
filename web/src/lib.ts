import type { Task } from "./types";

export function timestamp(value?: string) {
  return value && !value.startsWith("0001-") ? Date.parse(value) : NaN;
}
export function duration(start?: string, end?: string, now = Date.now()) {
  const first = timestamp(start);
  if (!Number.isFinite(first)) return "—";
  const last = timestamp(end);
  const seconds = Math.max(
    0,
    Math.floor(((Number.isFinite(last) ? last : now) - first) / 1000),
  );
  return seconds < 60
    ? `${seconds}s`
    : `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
}
export function time(value: string) {
  return new Date(value).toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}
export function short(id: string) {
  return id.slice(0, 8);
}
export async function request<T>(
  path: string,
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const res = await fetch(path, {
    method: body === undefined ? "GET" : "POST",
    headers: body === undefined ? {} : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  });
  if (!res.ok)
    throw new Error(
      (await res.text()).trim() || `Request failed (${res.status})`,
    );
  return res.json() as Promise<T>;
}

// Layer by longest dependency path; input order does not determine the graph.
export function layout(tasks: Task[], deps: Record<string, string[]>) {
  const ids = new Set(tasks.map((t) => t.ID));
  const levels = new Map<string, number>();
  const visiting = new Set<string>();
  const level = (id: string): number => {
    if (levels.has(id)) return levels.get(id)!;
    if (visiting.has(id)) return 0;
    visiting.add(id);
    const parents = (deps[id] ?? []).filter((p) => ids.has(p));
    const result = parents.length
      ? Math.max(...parents.map((p) => level(p) + 1))
      : 0;
    visiting.delete(id);
    levels.set(id, result);
    return result;
  };
  tasks.forEach((t) => level(t.ID));
  const columns: Task[][] = [];
  for (const t of tasks) {
    const n = levels.get(t.ID)!;
    (columns[n] ??= []).push(t);
  }
  const rows = Math.max(1, ...columns.map((c) => c.length));
  const height = rows * 128 + 32;
  const nodes = columns.flatMap((column, i) =>
    column.map((task, j) => ({
      task,
      x: 20 + i * 272,
      y: 20 + j * 128 + (rows - column.length) * 64,
    })),
  );
  return { nodes, width: Math.max(280, columns.length * 272), height };
}
