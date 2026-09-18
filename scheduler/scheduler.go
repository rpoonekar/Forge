package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/ronavpoonekar/forge/dag"
	"github.com/ronavpoonekar/forge/model"
	"github.com/ronavpoonekar/forge/store"
)

// Scheduler manages task scheduling and worker coordination.
type Scheduler struct {
	store *store.Store
}

func New(st *store.Store) *Scheduler {
	return &Scheduler{
		store: st,
	}
}

// StartLeaseChecker runs a background goroutine that periodically checks
// for dead workers and re-queues their tasks.
//
// This is the CORE distributed systems code in Forge.
//
// How it works:
//  1. Every `checkInterval` (e.g., 10 seconds), it wakes up
//  2. Queries the database for workers whose last_seen is older than `timeout`
//  3. For each stale worker:
//     a. Marks the worker as OFFLINE
//     b. Moves any RUNNING tasks back to QUEUED
//  4. Repeats until the context is cancelled (scheduler shutdown)
//
// The ctx parameter controls the lifecycle — when you cancel the context
// (e.g., on Ctrl+C), this goroutine stops cleanly.
//
// TODO (Step 3): Implement this method
//
// Skeleton:
//
//	func (s *Scheduler) StartLeaseChecker(ctx context.Context, checkInterval, timeout time.Duration) {
//	    ticker := time.NewTicker(checkInterval)
//	    defer ticker.Stop()
//
//	    log.Printf("Lease checker started (check every %s, timeout %s)", checkInterval, timeout)
//
//	    for {
//	        select {
//	        case <-ticker.C:
//	            // 1. Get stale workers from the store
//	            // 2. For each stale worker:
//	            //    a. Log that we detected a dead worker
//	            //    b. Call s.store.RequeueTasksForWorker(worker.ID)
//	            //    c. Log how many tasks were re-queued
//	            //    d. Mark the worker as OFFLINE:
//	            //       s.store.UpdateWorkerStatus(worker.ID, model.WorkerStatusOffline, "", 0)
//	        case <-ctx.Done():
//	            log.Println("Lease checker stopped")
//	            return
//	        }
//	    }
//	}
//
// Think about:
//   - The `select` statement waits for EITHER the ticker to fire OR the
//     context to be cancelled. Whichever happens first runs that case.
//   - This is idiomatic Go concurrency: a goroutine with a select loop.
//   - The caller runs this with: go s.StartLeaseChecker(ctx, 10*time.Second, 15*time.Second)
func (s *Scheduler) StartLeaseChecker(ctx context.Context, checkInterval, timeout time.Duration) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	log.Printf("Lease checker started (check every %s, timeout %s)", checkInterval, timeout)

	for {
		select {
		case <-ticker.C:
			staleworkers, err := s.store.GetStaleWorkers(timeout)
			if err != nil {
				log.Printf("Error checking stale workers: %v", err)
				continue
			}

			for _, worker := range staleworkers {
				log.Printf("Dead worker found (WorkerId: %s)", worker.ID)

				// Stage 6: Get the worker's running tasks and decide per-task
				tasks, err := s.store.GetRunningTasksForWorker(worker.ID)
				if err != nil {
					log.Printf("Error getting tasks for dead worker %s: %v", worker.ID, err)
					continue
				}

				for _, task := range tasks {
					// TODO (Step 3): Implement retry decision logic
					//
					// Check if the task has retries remaining:
					//   if task.RetryCount < task.MaxRetries {
					//       // Calculate exponential backoff delay: 2s * 2^retryCount
					//       delay := time.Duration(1<<task.RetryCount) * 2 * time.Second
					//       log.Printf("Retrying task %s (attempt %d/%d, delay %s)",
					//           task.ID, task.RetryCount+1, task.MaxRetries, delay)
					//       s.store.RetryTask(task.ID, delay)
					//   } else {
					//       // Max retries exceeded — fail permanently
					//       log.Printf("Task %s permanently FAILED (max retries %d exceeded)",
					//           task.ID, task.MaxRetries)
					//       s.store.FailTaskPermanently(task.ID)
					//   }
					if task.RetryCount < task.MaxRetries {
						delay := time.Duration(1<<task.RetryCount) * 2 * time.Second
						log.Printf("Retrying task %s (attempt %d/%d, delay %s)", task.ID, task.RetryCount+1, task.MaxRetries, delay)
						s.store.RetryTask(task.ID, delay)
					} else {
						log.Printf("Task %s permanently FAILED (max retries %d exceeded)", task.ID, task.MaxRetries)
						s.store.FailTaskPermanently(task.ID)
					}
				}

				s.store.UpdateWorkerStatus(worker.ID, model.WorkerStatusOffline, "", 0)
			}
		case <-ctx.Done():
			log.Println("Lease checker stopped")
			return
		}
	}
}

// --- Existing methods (unchanged from Stage 4) ---

func (s *Scheduler) RegisterWorker(workerID string) *model.Worker {
	worker, err := s.store.RegisterWorker(workerID)
	if err != nil {
		log.Printf("Error registering worker %s: %v", workerID, err)
		return nil
	}
	return worker
}

func (s *Scheduler) Submit(command string) *model.Task {
	task, err := s.store.CreateTask(command)
	if err != nil {
		log.Printf("Error creating task: %v", err)
		return nil
	}
	return task
}

// SubmitBuild validates the DAG dependencies and schedules a multi-task build.
func (s *Scheduler) SubmitBuild(tasks []model.TaskSpec) (*model.Build, error) {
	// Stage 8: Validate the DAG structure (checks for cycles & invalid deps)
	if _, err := dag.Validate(tasks); err != nil {
		return nil, err
	}

	return s.store.CreateBuild(tasks)
}

// GetBuild returns a build by ID.
func (s *Scheduler) GetBuild(id string) (*model.Build, error) {
	return s.store.GetBuild(id)
}

// GetAllBuilds returns all builds.
func (s *Scheduler) GetAllBuilds() []*model.Build {
	builds, err := s.store.GetAllBuilds()
	if err != nil {
		log.Printf("Error getting all builds: %v", err)
		return nil
	}
	return builds
}

func (s *Scheduler) NextTask(workerID string) *model.Task {
	task, err := s.store.AssignNextTask(workerID)
	if err != nil {
		log.Printf("Error assigning task to %s: %v", workerID, err)
		return nil
	}

	if task != nil {
		s.store.UpdateWorkerStatus(workerID, model.WorkerStatusBusy, task.ID, 0)
	} else {
		s.store.UpdateWorkerStatus(workerID, model.WorkerStatusIdle, "", 0)
	}

	return task
}

func (s *Scheduler) CompleteTask(taskID string, workerID string, output string, exitCode int) {
	if err := s.store.CompleteTask(taskID, output, exitCode); err != nil {
		log.Printf("Error completing task %s: %v", taskID, err)
	}
	if err := s.store.UpdateWorkerStatus(workerID, model.WorkerStatusIdle, "", 1); err != nil {
		log.Printf("Error updating worker %s: %v", workerID, err)
	}

	// Stage 8: If this task was part of a DAG build, resolve dependencies in Go!
	task, _ := s.store.GetTask(taskID)
	if task != nil && task.BuildID != "" {
		s.resolveBuildDependencies(task.BuildID, taskID, exitCode)
	}
}

// resolveBuildDependencies orchestrates pipeline advancement using pure Go DAG logic.
func (s *Scheduler) resolveBuildDependencies(buildID string, completedTaskID string, exitCode int) {
	tasks, deps, err := s.store.GetBuildGraph(buildID)
	if err != nil {
		log.Printf("Error getting build graph for %s: %v", buildID, err)
		return
	}

	if exitCode == 0 {
		// Task succeeded: call your Go DAG algorithm to find which BLOCKED tasks can now run!
		readyIDs := dag.FindRunnableTasks(tasks, deps)
		if len(readyIDs) > 0 {
			log.Printf("Build %s: unlocking %d task(s) to QUEUED: %v", buildID, len(readyIDs), readyIDs)
			if err := s.store.SetTasksStatus(readyIDs, model.StatusQueued); err != nil {
				log.Printf("Error unlocking tasks for build %s: %v", buildID, err)
			}
		}
	} else {
		// Task failed: cancel downstream blocked tasks that depended on this failed task
		canceledIDs := dag.FindDownstreamBlockedTasks(completedTaskID, deps, tasks)
		if len(canceledIDs) > 0 {
			log.Printf("Build %s: canceling %d downstream task(s): %v", buildID, len(canceledIDs), canceledIDs)
			if err := s.store.SetTasksStatus(canceledIDs, model.StatusCanceled); err != nil {
				log.Printf("Error canceling tasks for build %s: %v", buildID, err)
			}
		}
	}

	// Update overall build status (PENDING / RUNNING / SUCCEEDED / FAILED)
	if err := s.store.UpdateBuildStatus(completedTaskID); err != nil {
		log.Printf("Error updating build status for %s: %v", buildID, err)
	}
}

func (s *Scheduler) GetTask(id string) *model.Task {
	task, err := s.store.GetTask(id)
	if err != nil {
		log.Printf("Error getting task %s: %v", id, err)
		return nil
	}
	return task
}

func (s *Scheduler) GetAllTasks() []*model.Task {
	tasks, err := s.store.GetAllTasks()
	if err != nil {
		log.Printf("Error getting tasks: %v", err)
		return nil
	}
	return tasks
}

func (s *Scheduler) GetAllWorkers() []*model.Worker {
	workers, err := s.store.GetAllWorkers()
	if err != nil {
		log.Printf("Error getting workers: %v", err)
		return nil
	}
	return workers
}
