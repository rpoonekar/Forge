package model

import "time"

// Worker represents a registered worker in the cluster.
// The scheduler tracks all workers that have connected and
// records what each worker is currently doing.
type Worker struct {
	ID           string
	Status       WorkerStatus
	CurrentTask  string // ID of the task this worker is currently running ("" if idle)
	RegisteredAt time.Time
	LastSeen     time.Time // updated whenever the worker makes any gRPC call
	TasksRun     int       // total number of tasks this worker has completed
}

// WorkerStatus represents the current state of a worker.
type WorkerStatus string

const (
	WorkerStatusIdle    WorkerStatus = "IDLE"
	WorkerStatusBusy    WorkerStatus = "BUSY"
	WorkerStatusOffline WorkerStatus = "OFFLINE" // worker missed heartbeats and is presumed dead
)
