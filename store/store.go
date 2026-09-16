package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver — the underscore import registers it with database/sql

	"github.com/google/uuid"
	"github.com/ronavpoonekar/forge/model"
)

// Store wraps a PostgreSQL connection and provides methods for reading/writing
// tasks and workers.
//
// This replaces the in-memory maps from Stages 1-3. Every method here
// corresponds to a map operation the scheduler used to do directly:
//
//	s.tasks[id] = task          →  store.CreateTask(command)
//	task := s.tasks[id]         →  store.GetTask(id)
//	s.queue = s.queue[1:]       →  store.AssignNextTask(workerID)
//	s.workers[id] = worker      →  store.RegisterWorker(id)
//
// The big difference: data is on disk now. Kill the scheduler, restart it,
// and all tasks and workers are still there.
type Store struct {
	db *sql.DB
}

// New opens a connection to PostgreSQL and returns a Store.
//
// The connStr is a PostgreSQL connection string like:
//
//	"postgres://localhost:5432/forge?sslmode=disable"
//
// sql.Open doesn't actually connect — it just validates the driver name.
// db.Ping() actually tests the connection.
func New(connStr string) (*Store, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// --- Task methods ---

// CreateTask inserts a new task into the database with status QUEUED.
//
// This replaces:
//
//	id := GenerateID()
//	task := &model.Task{ID: id, Command: command, Status: "QUEUED", ...}
//	s.tasks[id] = task
//	s.queue = append(s.queue, id)
//
// In SQL, there's no separate "queue" — we just query for tasks WHERE status = 'QUEUED'
// ordered by created_at. The queue IS the query.
//
// TODO (Step 1): Implement this method
//
// SQL to use:
//
//	INSERT INTO tasks (id, command, status, created_at)
//	VALUES ($1, $2, $3, $4)
//
// Steps:
//  1. Generate a UUID: uuid.NewString()
//  2. Set created_at to time.Now()
//  3. Execute the INSERT with s.db.Exec(query, args...)
//  4. If err != nil, return nil, err
//  5. Build and return a *model.Task with the values you just inserted
//
// Helpful:
//
//	_, err := s.db.Exec("INSERT INTO tasks (id, command, status, created_at) VALUES ($1, $2, $3, $4)",
//	    id, command, "QUEUED", createdAt)
func (s *Store) CreateTask(command string) (*model.Task, error) {
	id := uuid.NewString()
	created_at := time.Now()
	status := model.StatusQueued

	_, err := s.db.Exec("INSERT INTO tasks (id, command, status, created_at) VALUES ($1, $2, $3, $4)", id, command, status, created_at)

	if err != nil {
		return nil, err
	}

	task := &model.Task{
		ID:        id,
		CreatedAt: created_at,
		Status:    status,
		Command:   command,
	}

	return task, nil
}

// GetTask retrieves a single task by ID.
//
// This replaces: task := s.tasks[id]
//
// TODO (Step 2): Implement this method
//
// SQL to use:
//
//	SELECT id, command, status, worker_id, created_at, started_at, ended_at, output, exit_code
//	FROM tasks WHERE id = $1
//
// Steps:
//  1. Execute the query with s.db.QueryRow(query, id)
//  2. Create a model.Task
//  3. Call row.Scan(&task.ID, &task.Command, ...) to populate it
//  4. Handle sql.ErrNoRows — return nil, nil (task not found, not an error)
//  5. Handle other errors — return nil, err
//  6. Return the task
//
// Important: started_at and ended_at can be NULL in the database (task hasn't
// started or ended yet). Use sql.NullTime to handle this:
//
//	var startedAt, endedAt sql.NullTime
//	row.Scan(..., &startedAt, &endedAt, ...)
//	if startedAt.Valid {
//	    task.StartedAt = startedAt.Time
//	}
func (s *Store) GetTask(id string) (*model.Task, error) {
	task := &model.Task{}
	var startedAt, endedAt sql.NullTime

	row := s.db.QueryRow("SELECT id, command, status, worker_id, created_at, started_at, ended_at, output, exit_code FROM tasks WHERE id = $1", id)
	err := row.Scan(&task.ID, &task.Command, &task.Status, &task.WorkerID, &task.CreatedAt, &startedAt, &endedAt, &task.Output, &task.ExitCode)

	if startedAt.Valid {
		task.StartedAt = startedAt.Time
	}
	if endedAt.Valid {
		task.EndedAt = endedAt.Time
	}

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return task, nil
}

// GetAllTasks retrieves all tasks.
//
// This replaces: iterating over s.tasks map
//
// TODO (Step 3): Implement this method
//
// SQL to use:
//
//	SELECT id, command, status, worker_id, created_at, started_at, ended_at, output, exit_code
//	FROM tasks ORDER BY created_at DESC
//
// Steps:
//  1. Execute the query with s.db.Query(query)
//  2. defer rows.Close()
//  3. Loop: for rows.Next() { ... row.Scan(...) ... }
//  4. Check rows.Err() after the loop
//  5. Return the slice of tasks
//
// This is similar to GetTask but you're scanning multiple rows instead of one.
func (s *Store) GetAllTasks() ([]*model.Task, error) {
	tasks := []*model.Task{}

	rows, err := s.db.Query("SELECT id, command, status, worker_id, created_at, started_at, ended_at, output, exit_code FROM tasks ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		task := &model.Task{}
		var startedAt, endedAt sql.NullTime

		rows.Scan(&task.ID, &task.Command, &task.Status, &task.WorkerID, &task.CreatedAt, &startedAt, &endedAt, &task.Output, &task.ExitCode)

		if startedAt.Valid {
			task.StartedAt = startedAt.Time
		}
		if endedAt.Valid {
			task.EndedAt = endedAt.Time
		}

		tasks = append(tasks, task)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return tasks, nil
}

// AssignNextTask atomically grabs the next QUEUED task and assigns it to a worker.
//
// This replaces the scheduler's NextTask():
//
//	id := s.queue[0]
//	s.queue = s.queue[1:]
//	task.Status = RUNNING
//	task.WorkerID = workerID
//	task.StartedAt = time.Now()
//
// In SQL, we do this in ONE query using UPDATE ... RETURNING:
//
//	UPDATE tasks
//	SET status = 'RUNNING', worker_id = $1, started_at = $2
//	WHERE id = (
//	    SELECT id FROM tasks WHERE status = 'QUEUED' ORDER BY created_at LIMIT 1
//	)
//	RETURNING id, command, status, worker_id, created_at, started_at, ended_at, output, exit_code
//
// This is powerful because:
//   - The subquery (SELECT ... LIMIT 1) finds the oldest QUEUED task
//   - The UPDATE atomically changes it to RUNNING and assigns the worker
//   - RETURNING gives us the updated row back
//   - If two workers call this at the same time, PostgreSQL ensures they
//     get DIFFERENT tasks (the database handles the concurrency for us!)
//
// TODO (Step 4): Implement this method
//
// Steps:
//  1. Run the query above with s.db.QueryRow(query, workerID, time.Now())
//  2. Scan the result into a model.Task (same as GetTask)
//  3. If sql.ErrNoRows — no queued tasks, return nil, nil
//  4. Return the task
func (s *Store) AssignNextTask(workerID string) (*model.Task, error) {
	query := "UPDATE tasks SET status = 'RUNNING', worker_id = $1, started_at = $2 WHERE id = (SELECT id FROM tasks WHERE status = 'QUEUED' ORDER BY created_at LIMIT 1) RETURNING id, command, status, worker_id, created_at, started_at, ended_at, output, exit_code"

	task := &model.Task{}
	var startedAt, endedAt sql.NullTime

	row := s.db.QueryRow(query, workerID, time.Now())
	err := row.Scan(&task.ID, &task.Command, &task.Status, &task.WorkerID, &task.CreatedAt, &startedAt, &endedAt, &task.Output, &task.ExitCode)

	if startedAt.Valid {
		task.StartedAt = startedAt.Time
	}
	if endedAt.Valid {
		task.EndedAt = endedAt.Time
	}

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return task, nil
}

// CompleteTask updates a task's status, output, and exit code.
//
// This replaces:
//
//	task.Status = SUCCEEDED/FAILED
//	task.Output = output
//	task.ExitCode = exitCode
//	task.EndedAt = time.Now()
//
// TODO (Step 5): Implement this method
//
// SQL to use:
//
//	UPDATE tasks SET status = $1, output = $2, exit_code = $3, ended_at = $4
//	WHERE id = $5
//
// Steps:
//  1. Determine status: if exitCode == 0, "SUCCEEDED", else "FAILED"
//  2. Execute the UPDATE
//  3. Return error if any
func (s *Store) CompleteTask(taskID string, output string, exitCode int) error {
	var status model.Status

	if exitCode == 0 {
		status = model.StatusSucceeded
	} else {
		status = model.StatusFailed
	}

	query := "UPDATE tasks SET status = $1, output = $2, exit_code = $3, ended_at = $4 WHERE id = $5"
	_, err := s.db.Exec(query, status, output, exitCode, time.Now(), taskID)

	if err != nil {
		return err
	}

	return nil
}

// --- Worker methods ---

// RegisterWorker inserts or updates a worker in the database.
//
// This replaces:
//
//	s.workers[workerID] = &model.Worker{...}
//
// We use INSERT ... ON CONFLICT (upsert) so that:
//   - First time: inserts a new row
//   - Re-registration: updates last_seen
//
// TODO (Step 6): Implement this method
//
// SQL to use:
//
//	INSERT INTO workers (id, status, registered_at, last_seen)
//	VALUES ($1, 'IDLE', $2, $2)
//	ON CONFLICT (id) DO UPDATE SET last_seen = $2
//	RETURNING id, status, current_task, registered_at, last_seen, tasks_run
//
// Steps:
//  1. Execute the query with s.db.QueryRow(query, workerID, time.Now())
//  2. Scan into a model.Worker
//  3. Return the worker
func (s *Store) RegisterWorker(workerID string) (*model.Worker, error) {
	worker := &model.Worker{}

	query := "INSERT INTO workers (id, status, registered_at, last_seen) VALUES ($1, 'IDLE', $2, $2) ON CONFLICT (id) DO UPDATE SET last_seen = $2 RETURNING id, status, current_task, registered_at, last_seen, tasks_run"
	row := s.db.QueryRow(query, workerID, time.Now())

	var registered_at, last_seen sql.NullTime
	err := row.Scan(&worker.ID, &worker.Status, &worker.CurrentTask, &registered_at, &last_seen, &worker.TasksRun)

	if registered_at.Valid {
		worker.RegisteredAt = registered_at.Time
	}
	if last_seen.Valid {
		worker.LastSeen = last_seen.Time
	}

	if err != nil {
		return nil, err
	}

	return worker, nil
}

// UpdateWorkerStatus updates a worker's status, current task, and last_seen.
//
// Called when:
//   - A worker takes a task (status = BUSY, currentTask = taskID)
//   - A worker completes a task (status = IDLE, currentTask = "")
//   - A worker polls with no work (just updates last_seen)
//
// TODO (Step 7): Implement this method
//
// SQL to use:
//
//	UPDATE workers SET status = $1, current_task = $2, last_seen = $3, tasks_run = tasks_run + $4
//	WHERE id = $5
//
// Parameters: status, currentTask, time.Now(), tasksRunDelta (0 or 1), workerID
func (s *Store) UpdateWorkerStatus(workerID string, status model.WorkerStatus, currentTask string, tasksRunDelta int) error {
	query := "UPDATE workers SET status = $1, current_task = $2, last_seen = $3, tasks_run = tasks_run + $4 WHERE id = $5"

	_, err := s.db.Exec(query, status, currentTask, time.Now(), tasksRunDelta, workerID)

	return err
}

// GetAllWorkers retrieves all workers.
//
// TODO (Step 8): Implement this method
//
// Same pattern as GetAllTasks but for the workers table.
func (s *Store) GetAllWorkers() ([]*model.Worker, error) {
	workers := []*model.Worker{}

	rows, err := s.db.Query("SELECT id, status, current_task, registered_at, last_seen, tasks_run FROM workers ORDER BY registered_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		worker := &model.Worker{}

		rows.Scan(&worker.ID, &worker.Status, &worker.CurrentTask, &worker.RegisteredAt, &worker.LastSeen, &worker.TasksRun)

		workers = append(workers, worker)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return workers, nil
}

// --- Stage 5: Heartbeat/Lease methods ---

// GetStaleWorkers returns workers whose last_seen is older than the given threshold.
// These workers are presumed dead.
//
// For example, if threshold is 15 seconds, this returns workers that haven't
// sent a heartbeat in the last 15 seconds.
//
// SQL:
//
//	SELECT id, status, current_task, registered_at, last_seen, tasks_run
//	FROM workers
//	WHERE last_seen < $1 AND status != 'OFFLINE'
//
// The $1 parameter is: time.Now().Add(-threshold)
// So if threshold is 15s and now is 12:00:15, we look for last_seen < 12:00:00
//
// TODO (Step 1): Implement this method
//
// Same scanning pattern as GetAllWorkers, but with a WHERE clause.
func (s *Store) GetStaleWorkers(threshold time.Duration) ([]*model.Worker, error) {
	workers := []*model.Worker{}

	rows, err := s.db.Query("SELECT id, status, current_task, registered_at, last_seen, tasks_run FROM workers WHERE last_seen < $1 AND status != 'OFFLINE'", time.Now().Add(-threshold))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		worker := &model.Worker{}

		rows.Scan(&worker.ID, &worker.Status, &worker.CurrentTask, &worker.RegisteredAt, &worker.LastSeen, &worker.TasksRun)

		workers = append(workers, worker)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return workers, nil
}

// RequeueTasksForWorker moves all RUNNING tasks assigned to a specific worker
// back to QUEUED so another worker can pick them up.
//
// This is called when a worker is detected as dead.
//
// SQL:
//
//	UPDATE tasks SET status = 'QUEUED', worker_id = '', started_at = NULL
//	WHERE worker_id = $1 AND status = 'RUNNING'
//
// Returns the number of tasks that were re-queued.
//
// TODO (Step 2): Implement this method
//
// Steps:
//  1. Execute the UPDATE
//  2. Get the number of affected rows: result.RowsAffected()
//  3. Return that count
func (s *Store) RequeueTasksForWorker(workerID string) (int64, error) {
	result, err := s.db.Exec("UPDATE tasks SET status = 'QUEUED', worker_id = '', started_at = NULL WHERE worker_id = $1 AND status = 'RUNNING'", workerID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
