package worker

import (
	"os/exec"
	"time"

	"github.com/ronavpoonekar/forge/scheduler"
)

// Worker pulls tasks from the scheduler and executes them directly on host.
// (Legacy Stage 1 implementation; current workers use Docker in executor/executor.go).
type Worker struct {
	id        string
	scheduler *scheduler.Scheduler
}

// New creates a new in-process Worker.
func New(id string, sched *scheduler.Scheduler) *Worker {
	return &Worker{
		id:        id,
		scheduler: sched,
	}
}

// Start begins the worker polling loop.
func (w *Worker) Start() {
	for {
		task := w.scheduler.NextTask(w.id)
		if task == nil {
			time.Sleep(1 * time.Second)
			continue
		}

		cmd := exec.Command("sh", "-c", task.Command)
		output, err := cmd.CombinedOutput()

		exitCode := 0
		if err != nil {
			exitCode = 1
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
		}

		w.scheduler.CompleteTask(task.ID, w.id, string(output), exitCode)
	}
}
