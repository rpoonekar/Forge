import { duration, layout } from "../lib";
import type { Task } from "../types";
import { Status } from "./Status";

export function TaskGraph({
  tasks,
  dependencies,
  selected,
  onSelect,
  now,
}: {
  tasks: Task[];
  dependencies: Record<string, string[]>;
  selected?: string;
  onSelect: (id: string) => void;
  now: number;
}) {
  const { nodes, width, height } = layout(tasks, dependencies);
  const byID = new Map(nodes.map((n) => [n.task.ID, n]));
  return (
    <div className="graph-scroll" aria-label="Task dependency graph">
      <div className="graph" style={{ width, height }}>
        <svg width={width} height={height} aria-hidden="true">
          <defs>
            <marker
              id="arrow"
              viewBox="0 0 10 10"
              refX="8"
              refY="5"
              markerWidth="5"
              markerHeight="5"
              orient="auto-start-reverse"
            >
              <path d="M 0 0 L 10 5 L 0 10 z" fill="#a6afba" />
            </marker>
          </defs>
          {nodes.flatMap((node) =>
            (dependencies[node.task.ID] ?? []).map((parent) => {
              const from = byID.get(parent);
              if (!from) return null;
              const x = from.x + 220,
                y = from.y + 48,
                endY = node.y + 48;
              return (
                <path
                  key={`${parent}-${node.task.ID}`}
                  d={`M ${x} ${y} C ${x + 26} ${y}, ${node.x - 26} ${endY}, ${node.x - 5} ${endY}`}
                  fill="none"
                  stroke="#b8c1cb"
                  strokeWidth="1.5"
                  markerEnd="url(#arrow)"
                />
              );
            }),
          )}
        </svg>
        {nodes.map(({ task, x, y }) => (
          <button
            key={task.ID}
            className={`task-node ${selected === task.ID ? "selected" : ""}`}
            style={{ left: x, top: y }}
            onClick={() => onSelect(task.ID)}
            aria-pressed={selected === task.ID}
            aria-label={`${task.Name}, ${task.Status.toLowerCase()}, depends on ${(dependencies[task.ID] ?? []).map((id) => byID.get(id)?.task.Name ?? id).join(", ") || "no tasks"}`}
          >
            <span className="node-title">
              {task.Name}
              <span className="mono muted">
                {duration(task.StartedAt, task.EndedAt, now)}
              </span>
            </span>
            <Status value={task.Status} />
            <span className="node-worker mono">
              {task.WorkerID ||
                (task.Status === "BLOCKED"
                  ? "Waiting on dependencies"
                  : "No worker assigned")}
              {task.RetryCount > 0 && ` · retry ${task.RetryCount}`}
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}
