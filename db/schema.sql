-- Forge database schema
-- Run this to set up the database:
--   createdb forge
--   psql forge < db/schema.sql

-- Tasks table: stores all task state (replaces the in-memory tasks map)
CREATE TABLE IF NOT EXISTS tasks (
    id            TEXT PRIMARY KEY,
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

-- Workers table: stores registered workers (replaces the in-memory workers map)
CREATE TABLE IF NOT EXISTS workers (
    id             TEXT PRIMARY KEY,
    status         TEXT NOT NULL DEFAULT 'IDLE',
    current_task   TEXT DEFAULT '',
    registered_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    tasks_run      INTEGER DEFAULT 0
);

-- Index on task status for fast queue lookups
-- When the scheduler calls "give me the next QUEUED task", this index
-- lets PostgreSQL find it instantly instead of scanning every row.
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);

