# Forge

> A fault-tolerant distributed CI execution engine written in Go.

Forge is a miniature execution platform inspired by the worker infrastructure behind modern CI systems (e.g., GitHub Actions, CircleCI). It accepts dependency-aware build pipelines (DAGs), schedules tasks across independent workers, isolates execution in ephemeral Docker containers, and recovers interrupted work when workers fail.

The project is intentionally focused on the execution plane: scheduling, worker coordination, durable state, failure recovery, and isolated workloads.

Interact with Forge through the [browser dashboard](#quick-start) or the
[HTTP API from your terminal](#optional-submit-workloads-through-the-api).
Both use the same scheduler, workers, and PostgreSQL database.

## Why Forge

CI systems must coordinate unreliable workers without losing work. Forge explores that problem with a deliberately small, inspectable architecture:

- **Concurrent Go workers** register with the scheduler and pull work over gRPC.
- **PostgreSQL-backed state** keeps task, build, and worker state durable across scheduler restarts.
- **Lease-based failure detection** identifies workers that stop heartbeating and reassigns their in-flight work with exponential backoff.
- **Ephemeral Docker containers** isolate commands, capture stdout/stderr, and make task environments reproducible.
- **Live React dashboard** submits demo and custom builds, visualizes dependency graphs, inspects task output, and demonstrates recovery with an opt-in worker interruption control.
- **Dynamic DAG scheduling** runs tasks in parallel waves, unlocking downstream tasks only after dependencies succeed, while rejecting cyclic build definitions.

## Architecture

```text
                         HTTP (REST + WebSocket)
   Browser ─────────────────────────────────────────────┐
                                                       v
                                             ┌──────────────────┐
                                             │   Go Scheduler   │
                                             │  - DAG resolver  │
                                             │  - leases        │
                                             │  - retry policy  │
                                             └────────┬─────────┘
                                                      │ gRPC (:50051)
                                      ┌───────────────┼───────────────┐
                                      v               v               v
                                ┌──────────┐    ┌──────────┐    ┌──────────┐
                                │ Worker 1 │    │ Worker 2 │    │ Worker N │
                                └────┬─────┘    └────┬─────┘    └────┬─────┘
                                     │ Docker        │ Docker        │ Docker
                                     v               v               v
                              Ephemeral build containers execute isolated tasks

                                      ┌──────────────────────────────┐
                                      │          PostgreSQL          │
                                      │ tasks, builds, dependencies  │
                                      └──────────────────────────────┘
```

## Failure Recovery

Each assigned task is associated with a worker lease. Workers periodically renew their liveness through heartbeats; if a worker stops responding, the scheduler marks it offline and recovers its running tasks.

```text
worker executes task
        │
        ├── heartbeats continue ──> task completes
        │
        └── heartbeats stop ──> lease expires ──> task re-queued
                                                   │
                                                   └── retry with backoff ──> another worker executes it
```

Tasks use at-least-once execution semantics: a task may be reassigned after a failed worker, so workloads should be idempotent when side effects matter.

## Dependency-Aware Builds

Builds are expressed as a directed acyclic graph (DAG). Forge uses Kahn's algorithm for pre-flight validation and cycle detection, then dynamically transitions tasks from `BLOCKED` to `QUEUED` as prerequisites succeed.

```text
       ┌─▶ lint ──┐
start ─┤          ├─▶ compile ──▶ deploy
       └─▶ test ──┘
```

- `lint` and `test` execute **concurrently** across available workers.
- `compile` waits until **both** upstream tasks succeed.
- If `test` fails, `compile` and `deploy` are automatically marked `CANCELED`.
- Cyclic definitions (e.g. `A ──▶ B ──▶ A`) are rejected at submission time with `HTTP 400`.

## Quick Start

Run the commands below from the `forge/` repository root. Start PostgreSQL and
Docker Desktop first. The scheduler serves the dashboard itself, so normal use
does not require a separate frontend server.

### Prerequisites

- **Go** matching `go.mod` (currently 1.27.1)
- **Node.js** 22.12+ (22 LTS) or 24+ and npm (only needed for the dashboard)
- **PostgreSQL** running locally (`postgres://localhost:5432/forge`)
- **Docker Desktop** running locally (Apple Silicon or Intel)
- Pull the base execution image:
  ```bash
  docker pull alpine:latest
  ```

### 1. Initialize the Database

For a new installation, create the database once:

```bash
createdb forge
```

Apply the schema to your local `forge` database:

```bash
psql forge < db/schema.sql
```

If the `forge` database already exists, skip `createdb` and run only the schema command.
It preserves existing builds, tasks, and workers, and adds the two queue-timing
columns. Apply this update even if you plan to use only the HTTP API.

### 2. Build the Dashboard

```bash
cd web
npm ci
npm run build
cd ..
```

Do this on first setup and rebuild after frontend changes. If you only want the
HTTP API, skip this step; the scheduler and workers run without dashboard assets.

### 3. Start the Scheduler

In **Terminal 1**, from the repository root:

```bash
go run ./cmd/scheduler --demo-controls
```

Open **http://127.0.0.1:8080**. The scheduler serves the built dashboard and REST /
WebSocket API on `127.0.0.1:8080`, and gRPC on `127.0.0.1:50051`.
`--demo-controls` enables the worker interruption button; omit it to disable that control.

Set `DATABASE_URL` to use a different database. `--http`, `--grpc`, and `--web-dir`
can override the listen addresses and dashboard asset directory. Run these commands
from the repository root.

### 4. Start Workers

Open two more terminals and change to the same repository root in each.
In **Terminal 2**:

```bash
go run ./cmd/worker --id worker-1 --image alpine:latest --demo-control
```

In **Terminal 3**:

```bash
go run ./cmd/worker --id worker-2 --image alpine:latest --demo-control
```

Leave all three terminals running. Two workers let one recover work after the
other is interrupted. Omit `--demo-control` for workers you do not want the
dashboard to stop.

### 5. Use the Dashboard

Open **[http://127.0.0.1:8080](http://127.0.0.1:8080)** in your browser.

- **Submit demo build** runs a dependency pipeline, a deliberately failing pipeline,
  or 20 parallel tasks. Select a workload from the dropdown.
- **New build** lets you enter task names, commands, and dependencies without curl.
- Select a build and then a task to inspect its worker, latest attempt duration,
  retry count, command, and captured output.
- **Workers** shows availability, current tasks, last heartbeats, and completed counts.
- While `lint` and `test` are running, click **Kill random worker**. The scheduler
  prefers a busy, responsive worker that opted in with `--demo-control`. It stops on
  its next heartbeat, then the real lease checker detects the failure and recovers
  the task on a surviving worker. Allow roughly 15–30 seconds for detection and backoff.
  Restart the stopped worker with its original command to bring it back.

Task status updates are live; output is collected after execution completes.
Starting the services still happens in terminals, but submitting and inspecting
builds and running the failure demo now happen in the browser.

### Starting Again and Shutting Down

On subsequent launches, start PostgreSQL and Docker Desktop, then repeat the
scheduler and worker commands in steps 3–4. You do not need to recreate the
database, reapply the schema, or rebuild an unchanged dashboard.

To shut down Forge, press **Ctrl+C** in each worker terminal and then in the
scheduler terminal. Close the dashboard tab. PostgreSQL and Docker Desktop are
separate services; stop them separately if you are finished using them.

### Optional: Submit Workloads Through the API

The original API remains available whether or not you build or open the dashboard.
Complete database setup and start the scheduler and workers as above; the demo
flags can be omitted for ordinary API use. The examples below use `curl` and,
where shown, `jq` to format responses.

#### Option A: Submit a Single Standalone Task
```bash
curl -X POST -d '{"command":"echo hello from standalone"}' http://localhost:8080/submit
```

#### Option B: Submit a Multi-Task Pipeline (DAG)
```bash
curl -s -X POST http://localhost:8080/build \
  -H 'Content-Type: application/json' \
  -d '{
    "tasks": [
      {"name": "lint", "command": "sleep 2 && echo lint-passed", "depends_on": []},
      {"name": "test", "command": "sleep 3 && echo test-passed", "depends_on": []},
      {"name": "compile", "command": "sleep 2 && echo compile-passed", "depends_on": ["lint", "test"]},
      {"name": "deploy", "command": "echo deploy-passed", "depends_on": ["compile"]}
    ]
  }' | jq .
```

#### Option C: Test Cycle Rejection (Verification)
```bash
curl -i -X POST http://localhost:8080/build \
  -H 'Content-Type: application/json' \
  -d '{
    "tasks": [
      {"name": "A", "command": "echo A", "depends_on": ["B"]},
      {"name": "B", "command": "echo B", "depends_on": ["A"]}
    ]
  }'
```
*(Returns `HTTP/1.1 400 Bad Request: Invalid build DAG: cycle detected in task dependencies`)*

### Optional: Inspect Through the API

```bash
# View all tasks and their outputs
curl -s http://localhost:8080/tasks | jq '.[] | {name: .Name, status: .Status, worker: .WorkerID, output: .Output}'

# View all builds
curl -s http://localhost:8080/builds | jq .

# View a specific build by ID
curl -s http://localhost:8080/build/<build-id> | jq .

# View registered workers
curl -s http://localhost:8080/workers | jq .
```

## Project Structure

```text
api/               REST routes, demo workloads, and bounded WebSocket broadcast hub
cmd/
  scheduler/       Scheduler entry point and HTTP / gRPC server lifecycle
  worker/          Worker entry point, registration, and heartbeat loop
dag/               DAG validation (Kahn's algorithm), cycle detection, and runtime resolution
db/                PostgreSQL unified schema (schema.sql)
executor/          Docker container lifecycle (create, start, wait, log demux, remove)
model/             Task, worker, build, and pipeline data models
proto/             Protocol Buffer definitions and generated gRPC stubs
scheduler/         Task scheduler, lease checker, retry policy, and failure recovery
store/             PostgreSQL persistence and consistent dashboard snapshots
web/               React / TypeScript UI (Vite) and components
```

## Design Decisions

| Decision | Rationale |
| --- | --- |
| **gRPC for scheduler-worker traffic** | Strongly typed protobuf contracts, efficient multiplexed HTTP/2 streaming. |
| **PostgreSQL as source of truth** | Durable state surviving scheduler crashes, atomic transactional updates. |
| **Pull-based workers** | Workers pull when ready, preventing head-of-line blocking and load imbalances. |
| **Heartbeats and leases** | Bounded failure detection for crashed workers without distributed consensus overhead. |
| **Docker execution per task** | Ephemeral, clean-room Linux containers preventing host pollution and cross-task contamination. |
| **Dynamic DAG scheduling in Go** | Core graph logic lives in Go memory for rapid scheduling, using Postgres strictly for durable state. |

## Development

```bash
# Run unit tests
go test ./...

# Verify code formatting and correctness
go vet ./...
```

For UI development, keep the scheduler running on port 8080 and run:

```bash
cd web
npm ci
npm run dev
```

Open the Vite URL printed in the terminal. Vite proxies `/api` and WebSocket traffic
to the scheduler; no cross-origin configuration is needed. Rebuild `web/dist` when
returning to the Go-served dashboard.

Build and type-check the frontend with `cd web && npm run build`.

## Roadmap

- [x] Concurrent worker registration and gRPC coordination
- [x] Durable PostgreSQL task and worker state
- [x] Heartbeats, lease expiry, automatic reassignment, and retry backoff
- [x] Ephemeral Docker task execution
- [x] Dependency DAG scheduling, cycle detection, and dynamic unlocking
- [x] Live React / TypeScript dashboard, WebSocket updates, and worker failure demo
- [ ] Prometheus metrics, load testing, and graceful shutdown hardening
