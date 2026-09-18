# Forge

> A fault-tolerant distributed CI execution engine written in Go.

Forge is a miniature execution platform inspired by the worker infrastructure behind modern CI systems. It accepts dependency-aware builds, schedules tasks across independent workers, and recovers interrupted work when a worker disappears.

The project is intentionally focused on the execution plane: scheduling, worker coordination, durable state, failure recovery, and isolated workloads.

## Why Forge

CI systems must coordinate unreliable workers without losing work. Forge explores that problem with a deliberately small, inspectable architecture:

- **Concurrent Go workers** register with the scheduler and pull work over gRPC.
- **PostgreSQL-backed state** keeps task and worker state durable across scheduler restarts.
- **Lease-based failure detection** identifies workers that stop heartbeating and reassigns their in-flight work with exponential backoff.
- **Ephemeral Docker containers** isolate commands, capture output, and make task environments reproducible.
- **DAG scheduling** runs tasks only after their dependencies succeed and rejects cyclic build definitions.

## Architecture

```text
                         HTTP
   Client  ────────────────────────────────────────────┐
                                                        v
                                              ┌─────────────────┐
                                              │  Go Scheduler   │
                                              │  - DAG resolver │
                                              │  - leases       │
                                              │  - retry policy │
                                              └───────┬─────────┘
                                                      │ gRPC
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
                                      │ tasks, workers, retry state  │
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

Builds are expressed as a directed acyclic graph. Forge uses topological scheduling to release work only when every upstream task has succeeded.

```text
test ──> compile ──> package
```

If `test` fails, `compile` and `package` are never scheduled. Cyclic definitions are rejected before execution begins.

## Quick Start

### Prerequisites

- Go
- PostgreSQL
- Docker with a running Docker daemon

### 1. Create the database

```bash
createdb forge
psql forge < db/schema.sql
```

### 2. Start the scheduler

```bash
go run ./cmd/scheduler
```

The scheduler exposes an HTTP API on `:8080` and accepts gRPC workers on `:50051`.

### 3. Start workers

Run each worker in a separate terminal:

```bash
go run ./cmd/worker -id worker-1
go run ./cmd/worker -id worker-2
go run ./cmd/worker -id worker-3
```

### 4. Submit a build

```bash
curl -X POST http://localhost:8080/builds \
  -H 'Content-Type: application/json' \
  -d '{
    "tasks": [
      {"id": "test", "image": "golang:latest", "command": "go test ./..."},
      {"id": "compile", "image": "golang:latest", "command": "go build ./...", "depends_on": ["test"]},
      {"id": "package", "image": "alpine:latest", "command": "tar -czf app.tar.gz ./app", "depends_on": ["compile"]}
    ]
  }'
```

Inspect a build and the worker pool:

```bash
curl http://localhost:8080/builds/<build-id>
curl http://localhost:8080/workers
```

## Project Structure

```text
cmd/
  scheduler/       Scheduler entry point and HTTP API
  worker/          Worker entry point and execution loop
db/                PostgreSQL schema
model/             Task, worker, build, and state-machine models
proto/             gRPC/Protocol Buffer service contract
scheduler/         Scheduling, leases, retries, and DAG resolution
store/             PostgreSQL persistence layer
worker/            Container execution and log collection
```

## Design Decisions

| Decision | Rationale |
| --- | --- |
| gRPC for scheduler-worker traffic | Strongly typed contracts and efficient service-to-service communication. |
| PostgreSQL as the source of truth | Task state survives scheduler restarts and state transitions can be made atomically. |
| Pull-based workers | Workers request work when ready, naturally providing backpressure. |
| Heartbeats and leases | A bounded, practical way to detect unavailable workers and recover their work. |
| Docker per task | Isolates workloads and creates reproducible execution environments. |
| DAG scheduling | Models real build and workflow dependencies without scheduling blocked work. |

## Local Security Note

Forge is designed for local development and controlled workloads. Do not expose arbitrary command submission on a public endpoint; a hosted demo should restrict users to predefined, sandboxed build definitions.

## Development

```bash
go test ./...
go vet ./...
```

## Roadmap

- [x] Concurrent worker registration and gRPC coordination
- [x] Durable PostgreSQL task and worker state
- [x] Heartbeats, lease expiry, automatic reassignment, and retry backoff
- [x] Ephemeral Docker task execution
- [x] Dependency DAG scheduling and cycle detection
- [ ] Live build dashboard and WebSocket updates
- [ ] Prometheus metrics, load testing, and graceful shutdown hardening
