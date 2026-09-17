package model

import "time"

// Status represents the current state of a task in its lifecycle.
type Status string

const (
	StatusQueued    Status = "QUEUED"
	StatusRunning   Status = "RUNNING"
	StatusRetrying  Status = "RETRYING" // Stage 6: task is waiting to be retried
	StatusSucceeded Status = "SUCCEEDED"
	StatusFailed    Status = "FAILED"
)

const DefaultMaxRetries = 3

// Task represents a single unit of work to be executed.
type Task struct {
	ID          string
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
