package model

import "time"

// Status represents the current state of a task in its lifecycle.
type Status string

const (
	StatusBlocked   Status = "BLOCKED" // Stage 8: waiting for dependencies to complete
	StatusQueued    Status = "QUEUED"
	StatusRunning   Status = "RUNNING"
	StatusRetrying  Status = "RETRYING" // Stage 6: task is waiting to be retried
	StatusSucceeded Status = "SUCCEEDED"
	StatusFailed    Status = "FAILED"
	StatusCanceled  Status = "CANCELED" // Stage 8: dependency failed, task cannot run
)

const DefaultMaxRetries = 3

// Task represents a single unit of work to be executed.
type Task struct {
	ID          string
	BuildID     string // Stage 8: which build this task belongs to
	Name        string // Stage 8: task identifier in the build (e.g. "lint", "test")
	Command     string
	Status      Status
	WorkerID    string // which worker is running (or ran) this task
	CreatedAt   time.Time
	StartedAt   time.Time
	EndedAt     time.Time
	Output      string // captured stdout/stderr from execution
	ExitCode    int
	RetryCount  int       // Stage 6: how many times this task has been retried
	MaxRetries  int       // Stage 6: max retry attempts allowed (default: 3)
	NextRetryAt time.Time // Stage 6: when this task is eligible for retry
}
