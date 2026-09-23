-- Forge database schema
-- Run this to set up the database:
--   createdb forge
--   psql forge < db/schema.sql

-- Builds table: stores DAG build workflows
CREATE TABLE IF NOT EXISTS builds (
    id          TEXT PRIMARY KEY,
    status      TEXT NOT NULL DEFAULT 'PENDING',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at    TIMESTAMPTZ
);

-- Tasks table: stores all task state
CREATE TABLE IF NOT EXISTS tasks (
    id            TEXT PRIMARY KEY,
    build_id      TEXT REFERENCES builds(id) ON DELETE CASCADE,
    name          TEXT DEFAULT '',
    command       TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'QUEUED',
    worker_id     TEXT DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at    TIMESTAMPTZ,
    ended_at      TIMESTAMPTZ,
    output        TEXT DEFAULT '',
    exit_code     INTEGER DEFAULT 0,
    retry_count   INTEGER DEFAULT 0,
    max_retries   INTEGER DEFAULT 3,
    next_retry_at TIMESTAMPTZ
);

-- Task dependencies join table: tracks DAG relationships
-- task_id depends on parent_id (parent_id must SUCCEED before task_id can run)
CREATE TABLE IF NOT EXISTS task_dependencies (
    task_id    TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    parent_id  TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, parent_id)
);

-- Workers table: stores registered workers
CREATE TABLE IF NOT EXISTS workers (
    id             TEXT PRIMARY KEY,
    status         TEXT NOT NULL DEFAULT 'IDLE',
    current_task   TEXT DEFAULT '',
    registered_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    tasks_run      INTEGER DEFAULT 0
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_build_id ON tasks(build_id);
CREATE INDEX IF NOT EXISTS idx_task_deps_parent ON task_dependencies(parent_id);

-- Dashboard queue timing. Re-running schema.sql upgrades existing databases.
-- Historical rows remain NULL rather than inventing queue latency samples.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS queued_at TIMESTAMPTZ;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS scheduling_latency_ms DOUBLE PRECISION;
