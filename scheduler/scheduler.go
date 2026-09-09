package scheduler

import (
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/ronavpoonekar/forge/model"
)

// Scheduler manages the task queue and assigns work.
//
// For Stage 1, it's simple:
//   - Maintains an in-memory queue of tasks
//   - Provides a way to submit new tasks
//   - Provides a way for the worker to grab the next available task
//   - Updates task status as tasks move through their lifecycle
//
// Think about:
//   - What data structure should the queue be? (slice? channel? map?)
//   - How do you generate unique task IDs?
//   - What happens if someone calls NextTask() and the queue is empty?
//   - This will need to be safe for concurrent access (mutex) — even in
//     Stage 1 since the HTTP handler and worker run in separate goroutines.
type Scheduler struct {
	mu    sync.Mutex
	tasks map[string]*model.Task // all tasks by ID
	queue []string               // IDs of tasks waiting to be picked up

	// TODO: You might want a channel here instead of (or in addition to)
	// the slice-based queue. Think about the tradeoffs:
	//   - Channel: worker can block waiting for work (no polling needed)
	//   - Slice + mutex: more control, but worker needs to poll or be notified
}

func GenerateID() string {
    return uuid.NewString()
}

// New creates a new Scheduler.
func New() *Scheduler {
	return &Scheduler{
		tasks: make(map[string]*model.Task),
		queue: []string{},
	}
}

// Submit adds a new task to the queue.
// It should:
//  1. Create a Task with a unique ID, the given command, and status QUEUED
//  2. Store it in the tasks map
//  3. Add its ID to the queue
//  4. Return the created task
//
// Remember: this will be called from the HTTP handler goroutine, while the
// worker is running in another goroutine. You need synchronization.
func (s *Scheduler) Submit(command string) *model.Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := GenerateID()

	task := &model.Task{
		ID: id,
		Command: command,
		Status: model.StatusQueued,
		CreatedAt: time.Now(),
	}

	s.queue = append(s.queue, id)
	s.tasks[id] = task
	return task
}

// NextTask returns the next task from the queue for a worker to execute.
// It should:
//  1. Check if the queue has any tasks
//  2. If yes: remove the first task from the queue, set its status to RUNNING,
//     record the start time, and return it
//  3. If no: return nil (nothing to do right now)
//
// The worker will call this in a loop.
func (s *Scheduler) NextTask() *model.Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.queue) == 0 {
		return nil
	}
	id := s.queue[0]
	task := s.tasks[id]

	task.StartedAt = time.Now()
	task.Status = model.StatusRunning
	
	s.queue = s.queue[1:]
	return task
}

// CompleteTask is called by the worker when it finishes executing a task.
// It should:
//  1. Look up the task by ID
//  2. Update its status to SUCCEEDED or FAILED based on the exit code
//  3. Store the output and exit code
//  4. Record the end time
func (s *Scheduler) CompleteTask(taskID string, output string, exitCode int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task := s.tasks[taskID]

	if exitCode == 0 {
		task.Status = model.StatusSucceeded
	} else {
		task.Status = model.StatusFailed
	}

	task.Output = output
	task.ExitCode = exitCode
	task.EndedAt = time.Now()
}

// GetTask returns a task by its ID (for the HTTP API to query status).
func (s *Scheduler) GetTask(id string) *model.Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.tasks[id]
}

// GetAllTasks returns all tasks (for the HTTP API to list them).
func (s *Scheduler) GetAllTasks() []*model.Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	allTasks := []*model.Task{}

	for _, task := range s.tasks {
		allTasks = append(allTasks, task)
	}

	return allTasks
}
