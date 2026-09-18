package model

import "time"

// Status represents the current state of a task in its lifecycle.
type Status string

const (
	StatusBlocked   Status = "BLOCKED"   // waiting for upstream dependencies to complete
	StatusQueued    Status = "QUEUED"    // ready for worker assignment
	StatusRunning   Status = "RUNNING"   // actively executing on a worker
	StatusRetrying  Status = "RETRYING"  // waiting for exponential backoff before retry
	StatusSucceeded Status = "SUCCEEDED" // completed with exit code 0
	StatusFailed    Status = "FAILED"    // completed with non-zero exit code or retries exhausted
	StatusCanceled  Status = "CANCELED"  // upstream dependency failed; execution skipped
)

const DefaultMaxRetries = 3

// Task represents a single unit of work to be executed.
type Task struct {
	ID          string
	BuildID     string // parent build ID (empty if standalone task)
	Name        string // pipeline stage name (e.g. "lint", "test")
	Command     string
	Status      Status
	WorkerID    string // worker currently running (or that ran) this task
	CreatedAt   time.Time
	StartedAt   time.Time
	EndedAt     time.Time
	Output      string // captured stdout/stderr from container execution
	ExitCode    int
	RetryCount  int       // number of retry attempts executed so far
	MaxRetries  int       // maximum retry attempts allowed
	NextRetryAt time.Time // earliest timestamp eligible for retry
}
