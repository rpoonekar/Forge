import { duration, short } from "../lib";
import type { Snapshot } from "../types";
import { Status } from "./Status";

export function Workers({ state }: { state: Snapshot }) {
  const tasks = new Map(
    state.builds.flatMap((b) => (b.tasks ?? []).map((t) => [t.ID, t] as const)),
  );
  return (
    <section className="panel workers-panel">
      <div className="section-heading">
        <div>
          <h2>Worker cluster</h2>
          <p>
            Workers pull tasks independently. Heartbeats renew their leases
            every 5 seconds.
          </p>
        </div>
        <span className="count">{state.workers.length} registered</span>
      </div>
      {state.workers.length === 0 ? (
        <div className="empty">
          <h3>No workers connected</h3>
          <p>
            Start a worker to begin executing queued tasks. See the README for
            setup.
          </p>
        </div>
      ) : (
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>Worker</th>
                <th>Status</th>
                <th>Current task</th>
                <th>Last heartbeat</th>
                <th>Completed</th>
                <th>Demo control</th>
              </tr>
            </thead>
            <tbody>
              {state.workers.map((w) => (
                <tr key={w.ID}>
                  <td className="mono strong">{w.ID}</td>
                  <td>
                    <Status value={w.Status} />
                  </td>
                  <td>
                    {w.CurrentTask
                      ? (tasks.get(w.CurrentTask)?.Name ?? short(w.CurrentTask))
                      : "—"}
                  </td>
                  <td>
                    <span
                      className={`heartbeat ${w.Status === "OFFLINE" ? "offline" : ""}`}
                      aria-hidden="true"
                    />
                    {duration(
                      w.LastSeen,
                      undefined,
                      Date.parse(state.captured_at),
                    )}{" "}
                    ago
                  </td>
                  <td className="mono">{w.TasksRun}</td>
                  <td>
                    {state.stopping_workers.includes(w.ID)
                      ? w.Status === "OFFLINE"
                        ? "Stopped"
                        : "Stop requested"
                      : state.demo_workers.includes(w.ID)
                        ? "Enabled"
                        : "Disabled"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
