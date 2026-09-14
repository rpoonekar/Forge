package scheduler

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ronavpoonekar/forge/model"
)

// Scheduler manages the task queue, assigns work, and tracks workers.
type Scheduler struct {
	mu      sync.Mutex
	tasks   map[string]*model.Task   // all tasks by ID
	queue   []string                 // IDs of tasks waiting to be picked up
	workers map[string]*model.Worker // all registered workers by ID
}

func GenerateID() string {
	return uuid.NewString()
}

// New creates a new Scheduler.
func New() *Scheduler {
	return &Scheduler{
		tasks:   make(map[string]*model.Task),
		queue:   []string{},
		workers: make(map[string]*model.Worker),
	}
}

// RegisterWorker adds a worker to the scheduler's registry (or updates LastSeen
// if already registered).
//
// This is called when a worker first connects via the RegisterWorker RPC.
// The scheduler needs to know which workers exist so it can:
//   - Show the worker pool in the dashboard (Stage 9)
//   - Detect dead workers via heartbeat timeout (Stage 5)
//   - Report stats (how many workers, what they're doing)
//
// TODO (Step 1): Implement this method
//
// Steps:
//  1. Lock the mutex
//  2. Check if a worker with this ID already exists in s.workers
//  3. If not, create a new model.Worker with status IDLE, RegisteredAt = now, LastSeen = now
//  4. If yes, just update its LastSeen time
//  5. Store it in the map and return it
func (s *Scheduler) RegisterWorker(workerID string) *model.Worker {
	s.mu.Lock()
	defer s.mu.Unlock()

	worker, ok := s.workers[workerID]
	if ok {
		worker.LastSeen = time.Now()
	} else {
		worker = &model.Worker{
			ID: workerID,
			Status: model.WorkerStatusIdle,
			RegisteredAt: time.Now(),
			LastSeen : time.Now(),
		}
		s.workers[workerID] = worker
	}
	return worker
}

// Submit adds a new task to the queue.
func (s *Scheduler) Submit(command string) *model.Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := GenerateID()

	task := &model.Task{
		ID:        id,
		Command:   command,
		Status:    model.StatusQueued,
		CreatedAt: time.Now(),
	}

	s.queue = append(s.queue, id)
	s.tasks[id] = task
	return task
}

// NextTask returns the next task from the queue for a specific worker.
//
// Stage 3 change: now takes a workerID parameter so we can record which
// worker is running each task and update the worker's status to BUSY.
//
// TODO (Step 2): Modify this method
//
// Your existing queue logic (pop from front, set RUNNING, etc.) stays the same.
// Add these new steps after popping the task:
//  1. Set task.WorkerID = workerID
//  2. Look up the worker in s.workers
//  3. Set worker.Status = BUSY
//  4. Set worker.CurrentTask = task.ID
//  5. Update worker.LastSeen
//
// Also: when the queue is empty, still update the worker's LastSeen
// (so we know it's alive and polling even when there's no work).
func (s *Scheduler) NextTask(workerID string) *model.Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.queue) == 0 {
		// No work, but update LastSeen so we know this worker is alive
		if w, ok := s.workers[workerID]; ok {
			w.LastSeen = time.Now()
		}
		return nil
	}

	id := s.queue[0]
	task := s.tasks[id]

	task.StartedAt = time.Now()
	task.Status = model.StatusRunning
	task.WorkerID = workerID

	s.queue = s.queue[1:]

	worker := s.workers[workerID]

	worker.CurrentTask = task.ID
	worker.LastSeen = time.Now()
	worker.Status = model.WorkerStatusBusy

	return task
}

// CompleteTask is called by the worker when it finishes executing a task.
//
// Stage 3 change: also updates the worker's state back to IDLE.
//
// TODO (Step 3): Add worker state update
//
// After your existing logic (set status, output, exit code, end time), add:
//  1. Look up the worker by task.WorkerID in s.workers
//  2. Set worker.Status = IDLE
//  3. Set worker.CurrentTask = ""
//  4. Increment worker.TasksRun
//  5. Update worker.LastSeen
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

	worker := s.workers[task.WorkerID]

	worker.LastSeen = time.Now()
	worker.CurrentTask = ""
	worker.Status = model.WorkerStatusIdle
	worker.TasksRun++
}

// GetTask returns a task by its ID (for the HTTP API).
func (s *Scheduler) GetTask(id string) *model.Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.tasks[id]
}

// GetAllTasks returns all tasks (for the HTTP API).
func (s *Scheduler) GetAllTasks() []*model.Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	allTasks := []*model.Task{}
	for _, task := range s.tasks {
		allTasks = append(allTasks, task)
	}
	return allTasks
}

// GetAllWorkers returns all registered workers (for the HTTP API).
//
// TODO (Step 4): Implement this method
//
// Same pattern as GetAllTasks — lock mutex, iterate s.workers map, return a slice.
func (s *Scheduler) GetAllWorkers() []*model.Worker {
	s.mu.Lock()
	defer s.mu.Unlock()

	allWorkers := []*model.Worker{}
	for _, worker := range s.workers {
		allWorkers = append(allWorkers, worker)
	}
	return allWorkers
}
