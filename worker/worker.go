package worker

import (
	"os/exec"
	"time"

	"github.com/ronavpoonekar/forge/scheduler"
)

// Worker pulls tasks from the scheduler and executes them.
//
// For Stage 1, the worker:
//   - Runs in a goroutine inside the same process as the scheduler
//   - Loops forever, asking the scheduler for work
//   - When it gets a task, executes the command using os/exec
//   - Reports the result (output + exit code) back to the scheduler
//   - If no work available, waits briefly before checking again
//
// Think about:
//   - How do you run a shell command in Go? (hint: os/exec, exec.Command)
//   - How do you capture both stdout and stderr?
//   - What does the exit code tell you? (0 = success, non-zero = failure)
//   - How do you avoid busy-looping when there's no work?
//     (hint: time.Sleep, or better yet, use a channel from the scheduler)
type Worker struct {
	id        string
	scheduler *scheduler.Scheduler
}

// New creates a new Worker.
func New(id string, sched *scheduler.Scheduler) *Worker {
	return &Worker{
		id:        id,
		scheduler: sched,
	}
}

// Start begins the worker loop. This should run in a goroutine.
//
// The basic loop:
//
//	for {
//	    task := s.scheduler.NextTask()
//	    if task == nil {
//	        // no work — wait a bit and try again
//	        continue
//	    }
//	    // execute the task command
//	    // capture output and exit code
//	    // report back to scheduler
//	}
//
// To execute a shell command:
//
//	cmd := exec.Command("sh", "-c", task.Command)
//	output, err := cmd.CombinedOutput()
//	exitCode := cmd.ProcessState.ExitCode()
//
// Later (Stage 5+), this will accept a context.Context for graceful shutdown.
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
			// If process actually ran, get the real exit code
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
		}

		w.scheduler.CompleteTask(task.ID, string(output), exitCode)
	}
}
