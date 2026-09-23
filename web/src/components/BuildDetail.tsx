import { useEffect, useState } from "react";
import { duration, request, short, time } from "../lib";
import type { Build, Task } from "../types";
import { Status } from "./Status";
import { TaskGraph } from "./TaskGraph";

export function BuildDetail({
  build,
  dependencies,
  now,
}: {
  build: Build;
  dependencies: Record<string, string[]>;
  now: number;
}) {
  const tasks = build.tasks ?? [];
  const [selected, setSelected] = useState<string>();
  const task = tasks.find((t) => t.ID === selected) ?? tasks[0];
  const [output, setOutput] = useState<{
    id: string;
    text: string;
    error?: string;
  }>();
  const [reload, setReload] = useState(0);
  useEffect(() => {
    if (!task) return;
    const controller = new AbortController();
    const id = task.ID;
    request<Task>(
      `/api/task/${encodeURIComponent(id)}`,
      undefined,
      controller.signal,
    )
      .then((t) => setOutput({ id, text: t.Output }))
      .catch((err) => {
        if (err.name !== "AbortError")
          setOutput({ id, text: "", error: err.message });
      });
    return () => controller.abort();
  }, [task?.ID, task?.Status, task?.RetryCount, reload]);
  const completed = tasks.filter((t) =>
    ["SUCCEEDED", "FAILED", "CANCELED"].includes(t.Status),
  ).length;
  return (
    <section className="build-detail" aria-label="Build detail">
      <header className="detail-header">
        <div>
          <div className="eyebrow">Build details</div>
          <h2 className="mono">
            {short(build.id)} <Status value={build.status} />
          </h2>
          <p>
            Started {time(build.created_at)}{" "}
            <span className="separator">/</span>{" "}
            {duration(build.created_at, build.ended_at, now)} elapsed
          </p>
        </div>
        <div className="completion">
          <strong>
            {completed}
            <span> / {tasks.length}</span>
          </strong>
          <span>tasks finished</span>
        </div>
      </header>
      <div
        className="progress"
        aria-label={`${completed} of ${tasks.length} tasks finished`}
      >
        <span
          style={{
            width: `${tasks.length ? (completed / tasks.length) * 100 : 0}%`,
          }}
        />
      </div>
      <div className="subheading">
        <h3>Task pipeline</h3>
        <span>Select a task to inspect its execution</span>
      </div>
      <TaskGraph
        tasks={tasks}
        dependencies={dependencies}
        selected={task?.ID}
        onSelect={setSelected}
        now={now}
      />
      {task && (
        <div className="task-inspector">
          <div className="section-heading">
            <h3>{task.Name}</h3>
            <Status value={task.Status} />
          </div>
          <dl className="task-facts">
            <div>
              <dt>Worker</dt>
              <dd className="mono">{task.WorkerID || "Unassigned"}</dd>
            </div>
            <div>
              <dt>Duration · latest attempt</dt>
              <dd>{duration(task.StartedAt, task.EndedAt, now)}</dd>
            </div>
            <div>
              <dt>Retries</dt>
              <dd>
                {task.RetryCount} / {task.MaxRetries}
              </dd>
            </div>
            <div>
              <dt>Exit code</dt>
              <dd>
                {["SUCCEEDED", "FAILED"].includes(task.Status)
                  ? task.ExitCode
                  : "—"}
              </dd>
            </div>
          </dl>
          {task.Status === "QUEUED" && task.RetryCount > 0 && (
            <p className="notice">
              Retry queued. Eligible after {time(task.NextRetryAt)}.
            </p>
          )}
          <div className="command mono">
            <span className="muted">$ </span>
            {task.Command}
          </div>
          <div className="subheading">
            <h4>Output</h4>
            <span>Collected when the task finishes</span>
          </div>
          {output?.id === task.ID && output.error ? (
            <div className="notice error" role="alert">
              {output.error}{" "}
              <button onClick={() => setReload((n) => n + 1)}>Retry</button>
            </div>
          ) : (
            <pre className="output">
              {output?.id !== task.ID
                ? "Loading output…"
                : output.text ||
                  (["RUNNING", "QUEUED", "BLOCKED"].includes(task.Status)
                    ? "Waiting for task output…"
                    : "No output recorded.")}
            </pre>
          )}
        </div>
      )}
    </section>
  );
}
