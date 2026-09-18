package model

import "time"

// BuildStatus represents the state of a multi-task build.
type BuildStatus string

const (
	BuildStatusPending   BuildStatus = "PENDING"   // Some tasks are queued or running
	BuildStatusRunning   BuildStatus = "RUNNING"   // At least one task is running
	BuildStatusSucceeded BuildStatus = "SUCCEEDED" // All tasks in the build succeeded
	BuildStatusFailed    BuildStatus = "FAILED"    // At least one task failed
)

// TaskSpec defines a task within a build submission.
type TaskSpec struct {
	Name      string   `json:"name"`
	Command   string   `json:"command"`
	DependsOn []string `json:"depends_on"`
}

// Build represents a DAG of tasks submitted together.
type Build struct {
	ID        string      `json:"id"`
	Status    BuildStatus `json:"status"`
	CreatedAt time.Time   `json:"created_at"`
	EndedAt   time.Time   `json:"ended_at,omitempty"`
	Tasks     []*Task     `json:"tasks,omitempty"`
}
