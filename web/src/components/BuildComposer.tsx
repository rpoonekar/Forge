import { useEffect, useRef, useState } from "react";
import type { Build, TaskSpec } from "../types";
import { request } from "../lib";

export function BuildComposer({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (build: Build) => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [tasks, setTasks] = useState([
    { name: "test", command: "echo 'Hello from Forge'", dependencies: "" },
  ]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    dialog.current?.showModal();
  }, []);
  const update = (
    index: number,
    field: keyof (typeof tasks)[number],
    value: string,
  ) =>
    setTasks((old) =>
      old.map((t, i) => (i === index ? { ...t, [field]: value } : t)),
    );
  return (
    <dialog
      ref={dialog}
      onCancel={(event) => {
        if (busy) event.preventDefault();
        else onClose();
      }}
      className="composer"
    >
      <form
        onSubmit={async (event) => {
          event.preventDefault();
          setBusy(true);
          setError("");
          const specs: TaskSpec[] = tasks.map((t) => ({
            name: t.name.trim(),
            command: t.command,
            depends_on: t.dependencies
              .split(",")
              .map((s) => s.trim())
              .filter(Boolean),
          }));
          try {
            onCreated(await request<Build>("/api/builds", { tasks: specs }));
          } catch (err) {
            setError((err as Error).message);
            setBusy(false);
          }
        }}
      >
        <header className="section-heading">
          <div>
            <h2>New build</h2>
            <p>Define tasks and the order they depend on each other.</p>
          </div>
          <button
            type="button"
            aria-label="Close new build"
            disabled={busy}
            onClick={onClose}
          >
            ×
          </button>
        </header>
        <p className="composer-note">
          Commands run in each worker’s configured Docker image. Dependencies
          control order; files are not shared between tasks.
        </p>
        <fieldset disabled={busy}>
          {tasks.map((task, i) => (
            <div className="task-form" key={i}>
              <div className="task-form-top">
                <span className="eyebrow">
                  Task {String(i + 1).padStart(2, "0")}
                </span>
                <button
                  type="button"
                  className="text-button"
                  disabled={tasks.length === 1}
                  onClick={() =>
                    setTasks((old) => old.filter((_, index) => index !== i))
                  }
                >
                  Remove
                </button>
              </div>
              <div className="form-pair">
                <label>
                  Name
                  <input
                    required
                    value={task.name}
                    onChange={(e) => update(i, "name", e.target.value)}
                    placeholder="compile"
                  />
                </label>
                <label>
                  Depends on{" "}
                  <span className="muted">(comma-separated names)</span>
                  <input
                    value={task.dependencies}
                    onChange={(e) => update(i, "dependencies", e.target.value)}
                    placeholder="lint, test"
                  />
                </label>
              </div>
              <label>
                Command
                <textarea
                  required
                  rows={2}
                  className="mono"
                  value={task.command}
                  onChange={(e) => update(i, "command", e.target.value)}
                />
              </label>
            </div>
          ))}
          <button
            type="button"
            disabled={tasks.length >= 100}
            onClick={() =>
              setTasks((old) => [
                ...old,
                { name: "", command: "", dependencies: "" },
              ])
            }
          >
            + Add task
          </button>
        </fieldset>
        {error && (
          <p className="notice error" role="alert">
            {error}
          </p>
        )}
        <footer className="dialog-footer">
          <button type="button" disabled={busy} onClick={onClose}>
            Cancel
          </button>
          <button className="primary" disabled={busy}>
            {busy ? "Submitting…" : "Submit build"}
          </button>
        </footer>
      </form>
    </dialog>
  );
}
