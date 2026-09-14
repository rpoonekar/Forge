package model

import "time"

// Status represents the current state of a task in its lifecycle.
// For Stage 1, we only need: Queued, Running, Succeeded, Failed.
// Later stages will add: Submitted, Assigned, Retrying.
type Status string

const (
	StatusQueued    Status = "QUEUED"
	StatusRunning   Status = "RUNNING"
	StatusSucceeded Status = "SUCCEEDED"
	StatusFailed    Status = "FAILED"
)

// Task represents a single unit of work to be executed.
// For Stage 1, a task is just a shell command (e.g., "echo hello", "sleep 2").
type Task struct {
	ID        string
	Command   string
	Status    Status
	WorkerID  string // which worker is running (or ran) this task
	CreatedAt time.Time
	StartedAt time.Time
	EndedAt   time.Time
	Output    string // captured stdout/stderr from execution
	ExitCode  int
}
