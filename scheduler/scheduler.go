package scheduler

import (
	"log"

	"github.com/ronavpoonekar/forge/model"
	"github.com/ronavpoonekar/forge/store"
)

// Scheduler manages task scheduling and worker coordination.
//
// Stage 4 change: the scheduler no longer holds any state in memory.
// All state lives in PostgreSQL via the Store. The mutex is gone too —
// the database handles concurrency (PostgreSQL serializes conflicting
// updates automatically).
//
// Compare to Stage 3:
//
//	Stage 3: s.mu.Lock(); task := s.tasks[id]; s.mu.Unlock()
//	Stage 4: task, err := s.store.GetTask(id)
//
// The methods below are thin wrappers around Store methods. They exist
// so that the gRPC server and HTTP handlers don't need to know about
// the Store directly — they still call scheduler.Submit(), etc.
type Scheduler struct {
	store *store.Store
}

// New creates a new Scheduler backed by the given Store.
func New(st *store.Store) *Scheduler {
	return &Scheduler{
		store: st,
	}
}

// RegisterWorker registers a worker (or updates its last_seen).
//
// TODO (Step 9): Implement this method
//
// Steps:
//  1. Call s.store.RegisterWorker(workerID)
//  2. Handle the error (log it, return nil if error)
//  3. Return the worker
func (s *Scheduler) RegisterWorker(workerID string) *model.Worker {
	worker, err := s.store.RegisterWorker(workerID)
	if err != nil {
		log.Printf("Error registering worker %s: %v", workerID, err)
		return nil
	}
	return worker
}

// Submit adds a new task to the queue.
//
// TODO (Step 9): Implement this method
//
// Steps:
//  1. Call s.store.CreateTask(command)
//  2. Handle the error
//  3. Return the task
func (s *Scheduler) Submit(command string) *model.Task {
	task, err := s.store.CreateTask(command)
	if err != nil {
		log.Printf("Error creating task: %v", err)
		return nil
	}
	return task
}

// NextTask assigns the next queued task to a worker.
//
// TODO (Step 10): Implement this method
//
// This needs to:
//  1. Call s.store.AssignNextTask(workerID)
//  2. If a task was returned, update the worker to BUSY:
//     s.store.UpdateWorkerStatus(workerID, model.WorkerStatusBusy, task.ID, 0)
//  3. If no task (nil), just update the worker's last_seen:
//     s.store.UpdateWorkerStatus(workerID, model.WorkerStatusIdle, "", 0)
//  4. Return the task (or nil)
func (s *Scheduler) NextTask(workerID string) *model.Task {
	task, err := s.store.AssignNextTask(workerID)
	if err != nil {
		log.Printf("Error assigning task to %s: %v", workerID, err)
		return nil
	}

	if task != nil {
		// Worker got a task — mark it as BUSY
		s.store.UpdateWorkerStatus(workerID, model.WorkerStatusBusy, task.ID, 0)
	} else {
		// No task available — just update last_seen
		s.store.UpdateWorkerStatus(workerID, model.WorkerStatusIdle, "", 0)
	}

	return task
}

// CompleteTask marks a task as completed and updates the worker.
//
// TODO (Step 11): Implement this method
//
// Steps:
//  1. Call s.store.CompleteTask(taskID, output, exitCode)
//  2. Update the worker back to IDLE with tasksRunDelta = 1:
//     s.store.UpdateWorkerStatus(workerID, model.WorkerStatusIdle, "", 1)
//
// Note: CompleteTask now needs workerID as a parameter (the gRPC server
// already sends it in the request).
func (s *Scheduler) CompleteTask(taskID string, workerID string, output string, exitCode int) {
	if err := s.store.CompleteTask(taskID, output, exitCode); err != nil {
		log.Printf("Error completing task %s: %v", taskID, err)
	}
	if err := s.store.UpdateWorkerStatus(workerID, model.WorkerStatusIdle, "", 1); err != nil {
		log.Printf("Error updating worker %s: %v", workerID, err)
	}
}

// GetTask returns a task by ID.
func (s *Scheduler) GetTask(id string) *model.Task {
	task, err := s.store.GetTask(id)
	if err != nil {
		log.Printf("Error getting task %s: %v", id, err)
		return nil
	}
	return task
}

// GetAllTasks returns all tasks.
func (s *Scheduler) GetAllTasks() []*model.Task {
	tasks, err := s.store.GetAllTasks()
	if err != nil {
		log.Printf("Error getting tasks: %v", err)
		return nil
	}
	return tasks
}

// GetAllWorkers returns all workers.
func (s *Scheduler) GetAllWorkers() []*model.Worker {
	workers, err := s.store.GetAllWorkers()
	if err != nil {
		log.Printf("Error getting workers: %v", err)
		return nil
	}
	return workers
}
