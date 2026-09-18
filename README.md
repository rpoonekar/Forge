# Forge

> A fault-tolerant distributed CI execution engine written in Go.

Forge is a miniature execution platform inspired by the worker infrastructure behind modern CI systems (e.g., GitHub Actions, CircleCI). It accepts dependency-aware build pipelines (DAGs), schedules tasks across independent workers, isolates execution in ephemeral Docker containers, and recovers interrupted work when workers fail.

The project is intentionally focused on the execution plane: scheduling, worker coordination, durable state, failure recovery, and isolated workloads.

## Why Forge

CI systems must coordinate unreliable workers without losing work. Forge explores that problem with a deliberately small, inspectable architecture:

- **Concurrent Go workers** register with the scheduler and pull work over gRPC.
- **PostgreSQL-backed state** keeps task, build, and worker state durable across scheduler restarts.
- **Lease-based failure detection** identifies workers that stop heartbeating and reassigns their in-flight work with exponential backoff.
- **Ephemeral Docker containers** isolate commands, capture stdout/stderr, and make task environments reproducible.
- **Dynamic DAG scheduling** runs tasks in parallel waves, unlocking downstream tasks only after dependencies succeed, while rejecting cyclic build definitions.

## Architecture

```text
                         HTTP (REST)
   Client ─────────────────────────────────────────────┐
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

### Prerequisites

- **Go** (1.22+)
- **PostgreSQL** running locally (`postgres://localhost:5432/forge`)
- **Docker Desktop** running locally (Apple Silicon or Intel)
- Pull the base execution image:
  ```bash
  docker pull alpine:latest
  ```

### 1. Initialize the Database

```bash
createdb forge
psql forge < db/schema.sql
```

### 2. Start the Scheduler

```bash
go run ./cmd/scheduler
```

The scheduler exposes a REST API on `:8080` and a gRPC server on `:50051`.

### 3. Start Workers

Run each worker in a separate terminal:

```bash
# Terminal 2
go run ./cmd/worker --id worker-1 --image alpine:latest

# Terminal 3
go run ./cmd/worker --id worker-2 --image alpine:latest
```

### 4. Submit Workload

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

### 5. Inspect the System

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
cmd/
  scheduler/       Scheduler entry point, REST API, and gRPC server
  worker/          Worker entry point, registration, and heartbeat loop
dag/               DAG validation (Kahn's algorithm), cycle detection, and runtime resolution
db/                PostgreSQL unified schema (schema.sql)
executor/          Docker container lifecycle (create, start, wait, log demux, remove)
model/             Task, worker, build, and pipeline data models
proto/             Protocol Buffer definitions and generated gRPC stubs
scheduler/         Task scheduler, lease checker, retry policy, and failure recovery
store/             PostgreSQL persistence layer
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

## Roadmap

- [x] Concurrent worker registration and gRPC coordination
- [x] Durable PostgreSQL task and worker state
- [x] Heartbeats, lease expiry, automatic reassignment, and retry backoff
- [x] Ephemeral Docker task execution
- [x] Dependency DAG scheduling, cycle detection, and dynamic unlocking
- [ ] Live build dashboard and WebSocket updates
- [ ] Prometheus metrics, load testing, and graceful shutdown hardening
