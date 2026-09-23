import { useState } from "react";
import { BuildComposer } from "./components/BuildComposer";
import { BuildDetail } from "./components/BuildDetail";
import { Status } from "./components/Status";
import { Workers } from "./components/Workers";
import { request, short, time } from "./lib";
import type { Build } from "./types";
import { useCluster } from "./useCluster";

export default function App() {
  const { state, connection, error } = useCluster();
  const [view, setView] = useState<"builds" | "workers">("builds");
  const [selected, setSelected] = useState<string>();
  const [filter, setFilter] = useState("all");
  const [kind, setKind] = useState("pipeline");
  const [busy, setBusy] = useState("");
  const [notice, setNotice] = useState("");
  const [actionError, setActionError] = useState("");
  const [composer, setComposer] = useState(false);
  const builds = state?.builds ?? [];
  const filtered = builds.filter(
    (b) =>
      filter === "all" ||
      (filter === "active"
        ? ["PENDING", "RUNNING"].includes(b.status)
        : b.status === filter),
  );
  const build = filtered.find((b) => b.id === selected) ?? filtered[0];
  const online =
    state?.workers.filter((w) => w.Status !== "OFFLINE").length ?? 0;
  const eligible = state?.workers.some(
    (w) =>
      w.Status !== "OFFLINE" &&
      Date.parse(state.captured_at) - Date.parse(w.LastSeen) <= 15000 &&
      state.demo_workers.includes(w.ID) &&
      !state.stopping_workers.includes(w.ID),
  );
  const live = connection === "live";
  const created = (b: Build) => {
    setSelected(b.id);
    setFilter("all");
    setView("builds");
    setComposer(false);
    setNotice(`Build ${short(b.id)} submitted.`)
  };
  const act = async (action: "submit" | "kill") => {
    setBusy(action);
    setActionError("");
    setNotice("");
    try {
      if (action === "submit")
        created(await request<Build>("/api/demo/builds", { kind }));
      else {
        const response = await request<{ worker_id: string }>(
          "/api/demo/kill-worker",
          {},
        );
        setNotice(
          `Stop requested for ${response.worker_id}. It stops on its next heartbeat; lease detection and recovery usually take 15–30 seconds. Restart that worker to bring it back.`,
        );
      }
    } catch (err) {
      setActionError((err as Error).message);
    } finally {
      setBusy("");
    }
  };
  return (
    <>
      <header className="topbar">
        <a className="brand" href="/" aria-label="Forge home">
          <span className="brand-mark">F</span>forge
          <span className="brand-caption">/ execution engine</span>
        </a>
        <div
          className="connection"
          title={
            state
              ? `Last snapshot ${time(state.captured_at)}`
              : "Waiting for the scheduler"
          }
        >
          <span className={live ? "live-dot" : "waiting-dot"} />
          {connection === "live"
            ? "Live connection"
            : connection === "connecting"
              ? "Connecting…"
              : "Reconnecting…"}
        </div>
      </header>
      <div className="nav-bar">
        <nav aria-label="Dashboard">
          <button
            className={view === "builds" ? "active" : ""}
            onClick={() => setView("builds")}
          >
            Builds
          </button>
          <button
            className={view === "workers" ? "active" : ""}
            onClick={() => setView("workers")}
          >
            Workers <span>{online}</span>
          </button>
        </nav>
        <span className="local-label">
          LOCAL CLUSTER <span className="separator">/</span> {online} online
        </span>
      </div>
      <main>
        <div className="page-heading">
          <div>
            <div className="eyebrow">Cluster overview</div>
            <h1>{view === "builds" ? "Build control" : "Worker cluster"}</h1>
            <p>
              {view === "builds"
                ? "Run pipelines, follow execution, and inspect recovery."
                : "Monitor availability and task assignments across the cluster."}
            </p>
          </div>
          <button
            className="primary"
            disabled={!live}
            onClick={() => setComposer(true)}
          >
            + New build
          </button>
        </div>
        {!live && (
          <div className="notice" role="status">
            {state
              ? "Connection interrupted. Showing the last received state; actions will resume when reconnected."
              : "Connecting to the scheduler. Cluster data will appear here when it is available."}
            {error && ` ${error}`}
          </div>
        )}
        {actionError && (
          <div className="notice error" role="alert">
            {actionError}
            <button className="text-button" onClick={() => setActionError("")}>
              Dismiss
            </button>
          </div>
        )}
        {notice && (
          <div className="notice" role="status">
            {notice}
            <button className="text-button" onClick={() => setNotice("")}>
              Dismiss
            </button>
          </div>
        )}
        <section className="metrics" aria-label="Cluster metrics">
          <Metric
            label="Queue depth"
            value={state?.metrics.queue_depth}
            detail={`${state?.metrics.running ?? 0} tasks running`}
          />
          <Metric
            label="Throughput"
            value={state?.metrics.throughput}
            unit="/ min"
            detail="Succeeded in the last 60s"
          />
          <Metric
            label="Total retries"
            value={state?.metrics.retries}
            detail="Across all retained tasks"
          />
          <Metric
            label="Scheduling latency"
            value={
              state?.metrics.scheduling_latency_ms == null
                ? undefined
                : Math.round(state.metrics.scheduling_latency_ms)
            }
            unit="ms"
            detail="Average queue wait · latest attempts"
          />
        </section>
        <section className="demo-toolbar" aria-label="Demo controls">
          <div className="demo-description">
            <strong>Try a workload</strong>
            <span>Predefined commands, isolated in containers.</span>
          </div>
          <div className="demo-actions">
            <select
              aria-label="Demo workload"
              value={kind}
              onChange={(e) => setKind(e.target.value)}
            >
              <option value="pipeline">Dependency pipeline</option>
              <option value="failure">Failing pipeline</option>
              <option value="parallel">20 parallel tasks</option>
            </select>
            <button onClick={() => act("submit")} disabled={!!busy || !live}>
              {busy === "submit" ? "Submitting…" : "Submit demo build"}
            </button>
            <span className="toolbar-divider" />
            <button
              className="danger"
              onClick={() => act("kill")}
              disabled={!!busy || !live || !state?.demo_enabled || !eligible}
              title={
                !state?.demo_enabled
                  ? "Start the scheduler with --demo-controls and workers with --demo-control"
                  : !eligible
                    ? "No responsive, opted-in workers are available"
                    : "Interrupt a worker and let lease recovery reassign its task"
              }
            >
              {busy === "kill" ? "Requesting…" : "Kill random worker"}
            </button>
          </div>
        </section>
        {state && state.workers.length === 0 && (
          <p className="notice">
            No workers are connected. Submitted builds will stay queued until a
            worker starts.
          </p>
        )}
        {view === "workers" ? (
          state ? (
            <Workers state={state} />
          ) : (
            <div className="panel empty">Waiting for cluster state…</div>
          )
        ) : (
          <div className="build-workspace panel">
            <aside className="build-list">
              <div className="build-list-header">
                <div>
                  <h2>
                    Builds <span className="count">{builds.length}</span>
                  </h2>
                  <span className="muted">Latest 50 submissions</span>
                </div>
                <select
                  aria-label="Filter builds"
                  value={filter}
                  onChange={(e) => setFilter(e.target.value)}
                >
                  <option value="all">All</option>
                  <option value="active">Active</option>
                  <option value="SUCCEEDED">Succeeded</option>
                  <option value="FAILED">Failed</option>
                </select>
              </div>
              <div className="build-rows">
                {filtered.length ? (
                  filtered.map((b) => (
                    <button
                      className={`build-row ${build?.id === b.id ? "selected" : ""}`}
                      aria-pressed={build?.id === b.id}
                      onClick={() => setSelected(b.id)}
                      key={b.id}
                    >
                      <span className="build-row-top">
                        <strong className="mono">{short(b.id)}</strong>
                        <Status value={b.status} />
                      </span>
                      <span className="build-row-bottom">
                        <span>{b.tasks?.length ?? 0} tasks</span>
                        <time
                          dateTime={b.created_at}
                          title={new Date(b.created_at).toLocaleString()}
                        >
                          {time(b.created_at)}
                        </time>
                      </span>
                    </button>
                  ))
                ) : (
                  <div className="list-empty">
                    {state
                      ? filter === "all"
                        ? "Your builds will appear here."
                        : "No builds match this filter."
                      : "Loading builds…"}
                  </div>
                )}
              </div>
            </aside>
            {build && state ? (
              <BuildDetail
                key={build.id}
                build={build}
                dependencies={state.dependencies}
                now={Date.parse(state.captured_at)}
              />
            ) : (
              <div className="empty workspace-empty">
                <span className="empty-glyph" aria-hidden="true">
                  ⌘
                </span>
                <h2>
                  {filter === "all"
                    ? "Ready for your first build"
                    : "No matching builds"}
                </h2>
                <p>
                  {filter === "all"
                    ? "Submit a demo pipeline to see tasks move through the cluster, or create a build of your own."
                    : "Choose another filter to inspect your builds."}
                </p>
                {filter === "all" && (
                  <button
                    disabled={!!busy || !live}
                    onClick={() => act("submit")}
                  >
                    Submit demo build
                  </button>
                )}
              </div>
            )}
          </div>
        )}
        <footer className="page-footer">
          <span>
            Forge <span className="separator">/</span> Distributed CI execution
          </span>
          <span>
            {state
              ? `Last update ${time(state.captured_at)}`
              : "Waiting for first update"}
          </span>
        </footer>
      </main>
      {composer && (
        <BuildComposer onClose={() => setComposer(false)} onCreated={created} />
      )}
    </>
  );
}
function Metric({
  label,
  value,
  detail,
  unit,
}: {
  label: string;
  value?: number;
  detail: string;
  unit?: string;
}) {
  return (
    <div className="metric">
      <span>{label}</span>
      <div className="metric-value">
        {value === undefined ? "—" : value.toLocaleString()}
        {unit && <small>{unit}</small>}
      </div>
      <span className="metric-detail">{detail}</span>
    </div>
  );
}
