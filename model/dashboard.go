package model

import "time"

// Snapshot is a consistent, bounded view of recent builds and the live cluster.
// Output is fetched separately through the task endpoint to keep broadcasts small.
type Snapshot struct {
	Builds          []*Build            `json:"builds"`
	Workers         []*Worker           `json:"workers"`
	Dependencies    map[string][]string `json:"dependencies"`
	Metrics         Metrics             `json:"metrics"`
	CapturedAt      time.Time           `json:"captured_at"`
	DemoEnabled     bool                `json:"demo_enabled"`
	DemoWorkers     []string            `json:"demo_workers"`
	StoppingWorkers []string            `json:"stopping_workers"`
}

type Metrics struct {
	QueueDepth          int      `json:"queue_depth"`
	Running             int      `json:"running"`
	Throughput          int      `json:"throughput"`            // Successful tasks in the trailing 60 seconds.
	Retries             int      `json:"retries"`               // Total retries across retained tasks.
	SchedulingLatencyMS *float64 `json:"scheduling_latency_ms"` // Latest assignment per task.
}
