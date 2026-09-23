package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"

	"github.com/google/uuid"
	"github.com/ronavpoonekar/forge/model"
)

// Store wraps a PostgreSQL connection and provides methods for reading and
// writing tasks, workers, builds, and dependency edges to persistent storage.
type Store struct {
	db *sql.DB
}

// New opens a connection to PostgreSQL and verifies connectivity.
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

// CreateTask inserts a new standalone task into the database with status QUEUED.
func (s *Store) CreateTask(command string) (*model.Task, error) {
	id := uuid.NewString()
	createdAt := time.Now()
	status := model.StatusQueued
	maxRetries := model.DefaultMaxRetries

	query := `INSERT INTO tasks (id, command, status, created_at, max_retries, queued_at)
	          VALUES ($1, $2, $3, $4, $5, $4)`
	_, err := s.db.Exec(query, id, command, status, createdAt, maxRetries)
	if err != nil {
		return nil, err
	}

	return &model.Task{
		ID:         id,
		CreatedAt:  createdAt,
		Status:     status,
		Command:    command,
		MaxRetries: maxRetries,
	}, nil
}

// GetTask retrieves a single task by ID.
func (s *Store) GetTask(id string) (*model.Task, error) {
	task := &model.Task{}
	var startedAt, endedAt, nextRetryAt sql.NullTime
	var buildID, name sql.NullString

	query := `SELECT id, build_id, name, command, status, worker_id, created_at,
	                 started_at, ended_at, output, exit_code, retry_count,
	                 max_retries, next_retry_at
	          FROM tasks WHERE id = $1`
	row := s.db.QueryRow(query, id)
	err := row.Scan(&task.ID, &buildID, &name, &task.Command, &task.Status, &task.WorkerID,
		&task.CreatedAt, &startedAt, &endedAt, &task.Output, &task.ExitCode,
		&task.RetryCount, &task.MaxRetries, &nextRetryAt)

	if buildID.Valid {
		task.BuildID = buildID.String
	}
	if name.Valid {
		task.Name = name.String
	}
	if startedAt.Valid {
		task.StartedAt = startedAt.Time
	}
	if endedAt.Valid {
		task.EndedAt = endedAt.Time
	}
	if nextRetryAt.Valid {
		task.NextRetryAt = nextRetryAt.Time
	}

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return task, nil
}

// GetAllTasks retrieves all tasks ordered by creation time descending.
func (s *Store) GetAllTasks() ([]*model.Task, error) {
	query := `SELECT id, build_id, name, command, status, worker_id, created_at,
	                 started_at, ended_at, output, exit_code, retry_count,
	                 max_retries, next_retry_at
	          FROM tasks ORDER BY created_at DESC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*model.Task
	for rows.Next() {
		task := &model.Task{}
		var startedAt, endedAt, nextRetryAt sql.NullTime
		var buildID, name sql.NullString

		if err := rows.Scan(&task.ID, &buildID, &name, &task.Command, &task.Status,
			&task.WorkerID, &task.CreatedAt, &startedAt, &endedAt, &task.Output,
			&task.ExitCode, &task.RetryCount, &task.MaxRetries, &nextRetryAt); err != nil {
			return nil, err
		}

		if buildID.Valid {
			task.BuildID = buildID.String
		}
		if name.Valid {
			task.Name = name.String
		}
		if startedAt.Valid {
			task.StartedAt = startedAt.Time
		}
		if endedAt.Valid {
			task.EndedAt = endedAt.Time
		}
		if nextRetryAt.Valid {
			task.NextRetryAt = nextRetryAt.Time
		}

		tasks = append(tasks, task)
	}

	return tasks, rows.Err()
}

// AssignNextTask atomically grabs the oldest eligible QUEUED task and assigns it
// to a worker.
//
// Uses a Common Table Expression (CTE) with FOR UPDATE SKIP LOCKED to prevent
// race conditions when multiple workers poll simultaneously.
func (s *Store) AssignNextTask(workerID string) (*model.Task, error) {
	query := `
		WITH next_task AS (
			SELECT id
			FROM tasks
			WHERE status = 'QUEUED' AND (next_retry_at IS NULL OR next_retry_at <= NOW())
			ORDER BY created_at
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE tasks
		SET status = 'RUNNING', worker_id = $1, started_at = $2,
            scheduling_latency_ms = CASE WHEN queued_at IS NOT NULL THEN GREATEST(0, EXTRACT(EPOCH FROM ($2::timestamptz - queued_at)) * 1000) END
		FROM next_task
		WHERE tasks.id = next_task.id
		RETURNING tasks.id, tasks.build_id, tasks.name, tasks.command, tasks.status,
		          tasks.worker_id, tasks.created_at, tasks.started_at, tasks.ended_at,
		          tasks.output, tasks.exit_code, tasks.retry_count, tasks.max_retries,
		          tasks.next_retry_at`

	task := &model.Task{}
	var startedAt, endedAt, nextRetryAt sql.NullTime
	var buildID, name sql.NullString

	row := s.db.QueryRow(query, workerID, time.Now())
	err := row.Scan(&task.ID, &buildID, &name, &task.Command, &task.Status, &task.WorkerID,
		&task.CreatedAt, &startedAt, &endedAt, &task.Output, &task.ExitCode,
		&task.RetryCount, &task.MaxRetries, &nextRetryAt)

	if buildID.Valid {
		task.BuildID = buildID.String
	}
	if name.Valid {
		task.Name = name.String
	}
	if startedAt.Valid {
		task.StartedAt = startedAt.Time
	}
	if endedAt.Valid {
		task.EndedAt = endedAt.Time
	}
	if nextRetryAt.Valid {
		task.NextRetryAt = nextRetryAt.Time
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
func (s *Store) CompleteTask(taskID string, workerID string, output string, exitCode int) error {
	var status model.Status
	if exitCode == 0 {
		status = model.StatusSucceeded
	} else {
		status = model.StatusFailed
	}

	query := `UPDATE tasks
	          SET status = $1, output = $2, exit_code = $3, ended_at = $4
	          WHERE id = $5 AND worker_id = $6 AND status = 'RUNNING'`
	result, err := s.db.Exec(query, status, output, exitCode, time.Now(), taskID, workerID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return fmt.Errorf("task is no longer assigned to worker %s", workerID)
	}
	return err
}

// --- Worker methods ---

// RegisterWorker inserts or updates a worker in the database using an upsert.
func (s *Store) RegisterWorker(workerID string) (*model.Worker, error) {
	worker := &model.Worker{}

	query := `INSERT INTO workers (id, status, registered_at, last_seen)
	          VALUES ($1, 'IDLE', $2, $2)
	          ON CONFLICT (id) DO UPDATE SET last_seen = $2
	          RETURNING id, status, current_task, registered_at, last_seen, tasks_run`
	row := s.db.QueryRow(query, workerID, time.Now())

	var registeredAt, lastSeen sql.NullTime
	err := row.Scan(&worker.ID, &worker.Status, &worker.CurrentTask, &registeredAt, &lastSeen, &worker.TasksRun)

	if registeredAt.Valid {
		worker.RegisteredAt = registeredAt.Time
	}
	if lastSeen.Valid {
		worker.LastSeen = lastSeen.Time
	}

	return worker, err
}

// UpdateWorkerStatus updates a worker's status, current task, and last_seen timestamp.
func (s *Store) UpdateWorkerStatus(workerID string, status model.WorkerStatus, currentTask string, tasksRunDelta int) error {
	query := `UPDATE workers
	          SET status = $1, current_task = $2,
	              last_seen = CASE WHEN $1 = 'OFFLINE' THEN last_seen ELSE $3 END,
	              tasks_run = tasks_run + $4
	          WHERE id = $5`
	_, err := s.db.Exec(query, status, currentTask, time.Now(), tasksRunDelta, workerID)
	return err
}

// GetAllWorkers retrieves all registered workers.
func (s *Store) GetAllWorkers() ([]*model.Worker, error) {
	query := `SELECT id, status, current_task, registered_at, last_seen, tasks_run
	          FROM workers ORDER BY registered_at DESC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workers []*model.Worker
	for rows.Next() {
		worker := &model.Worker{}
		if err := rows.Scan(&worker.ID, &worker.Status, &worker.CurrentTask,
			&worker.RegisteredAt, &worker.LastSeen, &worker.TasksRun); err != nil {
			return nil, err
		}
		workers = append(workers, worker)
	}

	return workers, rows.Err()
}

// --- Heartbeat and Lease recovery methods ---

// GetStaleWorkers returns workers whose last_seen timestamp is older than the threshold.
func (s *Store) GetStaleWorkers(threshold time.Duration) ([]*model.Worker, error) {
	query := `SELECT id, status, current_task, registered_at, last_seen, tasks_run
	          FROM workers
	          WHERE last_seen < $1 AND status != 'OFFLINE'`
	rows, err := s.db.Query(query, time.Now().Add(-threshold))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workers []*model.Worker
	for rows.Next() {
		worker := &model.Worker{}
		if err := rows.Scan(&worker.ID, &worker.Status, &worker.CurrentTask,
			&worker.RegisteredAt, &worker.LastSeen, &worker.TasksRun); err != nil {
			return nil, err
		}
		workers = append(workers, worker)
	}

	return workers, rows.Err()
}

// GetRunningTasksForWorker returns all RUNNING tasks assigned to a specific worker.
func (s *Store) GetRunningTasksForWorker(workerID string) ([]*model.Task, error) {
	query := `SELECT id, command, status, worker_id, created_at, started_at, ended_at,
	                 output, exit_code, retry_count, max_retries, next_retry_at
	          FROM tasks WHERE worker_id = $1 AND status = 'RUNNING'`
	rows, err := s.db.Query(query, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*model.Task
	for rows.Next() {
		task := &model.Task{}
		var startedAt, endedAt, nextRetryAt sql.NullTime

		if err := rows.Scan(&task.ID, &task.Command, &task.Status, &task.WorkerID,
			&task.CreatedAt, &startedAt, &endedAt, &task.Output, &task.ExitCode,
			&task.RetryCount, &task.MaxRetries, &nextRetryAt); err != nil {
			return nil, err
		}

		if startedAt.Valid {
			task.StartedAt = startedAt.Time
		}
		if endedAt.Valid {
			task.EndedAt = endedAt.Time
		}
		if nextRetryAt.Valid {
			task.NextRetryAt = nextRetryAt.Time
		}

		tasks = append(tasks, task)
	}

	return tasks, rows.Err()
}

// RetryTask increments a task's retry count and returns it to QUEUED status
// with a next_retry_at eligibility timestamp calculated via exponential backoff.
func (s *Store) RetryTask(taskID string, retryDelay time.Duration) error {
	query := `UPDATE tasks
	          SET status = 'QUEUED', worker_id = '', started_at = NULL,
	              retry_count = retry_count + 1, next_retry_at = $1, queued_at = $1, scheduling_latency_ms = NULL
	          WHERE id = $2`
	_, err := s.db.Exec(query, time.Now().Add(retryDelay), taskID)
	return err
}

// FailTaskPermanently marks a task as permanently FAILED when max retries is exceeded.
func (s *Store) FailTaskPermanently(taskID string) error {
	query := `UPDATE tasks SET status = 'FAILED', ended_at = $1, exit_code = -1, output = 'Worker lease expired; maximum retries exceeded.' WHERE id = $2`
	_, err := s.db.Exec(query, time.Now(), taskID)
	return err
}

// --- Build and DAG methods ---

// CreateBuild creates a new build and its DAG tasks in a single database transaction.
func (s *Store) CreateBuild(tasks []model.TaskSpec) (*model.Build, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	buildID := uuid.NewString()
	now := time.Now()

	_, err = tx.Exec("INSERT INTO builds (id, status, created_at) VALUES ($1, $2, $3)",
		buildID, model.BuildStatusPending, now)
	if err != nil {
		return nil, fmt.Errorf("failed to insert build: %w", err)
	}

	nameToID := make(map[string]string)
	for _, spec := range tasks {
		nameToID[spec.Name] = uuid.NewString()
	}

	var createdTasks []*model.Task
	for _, spec := range tasks {
		taskID := nameToID[spec.Name]

		initialStatus := model.StatusQueued
		if len(spec.DependsOn) > 0 {
			initialStatus = model.StatusBlocked
		}

		_, err := tx.Exec(`INSERT INTO tasks (id, build_id, name, command, status, created_at, max_retries, queued_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, CASE WHEN $5 = 'QUEUED' THEN $6::timestamptz ELSE NULL END)`,
			taskID, buildID, spec.Name, spec.Command, initialStatus, now, model.DefaultMaxRetries)
		if err != nil {
			return nil, fmt.Errorf("failed to insert task %s: %w", spec.Name, err)
		}

		createdTasks = append(createdTasks, &model.Task{
			ID:         taskID,
			BuildID:    buildID,
			Name:       spec.Name,
			Command:    spec.Command,
			Status:     initialStatus,
			CreatedAt:  now,
			MaxRetries: model.DefaultMaxRetries,
		})
	}

	// Insert edges after every task exists; submitted specs need not be topologically ordered.
	for _, spec := range tasks {
		for _, depName := range spec.DependsOn {
			if _, err := tx.Exec(`INSERT INTO task_dependencies (task_id, parent_id) VALUES ($1, $2)`, nameToID[spec.Name], nameToID[depName]); err != nil {
				return nil, fmt.Errorf("failed to insert task dependency: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit build: %w", err)
	}

	return &model.Build{
		ID:        buildID,
		Status:    model.BuildStatusPending,
		CreatedAt: now,
		Tasks:     createdTasks,
	}, nil
}

// GetBuild retrieves a build and all of its tasks.
func (s *Store) GetBuild(buildID string) (*model.Build, error) {
	b := &model.Build{}
	var endedAt sql.NullTime

	query := "SELECT id, status, created_at, ended_at FROM builds WHERE id = $1"
	err := s.db.QueryRow(query, buildID).Scan(&b.ID, &b.Status, &b.CreatedAt, &endedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if endedAt.Valid {
		b.EndedAt = endedAt.Time
	}

	taskQuery := `SELECT id, build_id, name, command, status, worker_id, created_at,
	                     started_at, ended_at, output, exit_code, retry_count, max_retries
	              FROM tasks WHERE build_id = $1 ORDER BY created_at ASC`
	rows, err := s.db.Query(taskQuery, buildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		t := &model.Task{}
		var buildIDCol, nameCol sql.NullString
		var startedAt, endedAtCol sql.NullTime
		if err := rows.Scan(&t.ID, &buildIDCol, &nameCol, &t.Command, &t.Status,
			&t.WorkerID, &t.CreatedAt, &startedAt, &endedAtCol, &t.Output,
			&t.ExitCode, &t.RetryCount, &t.MaxRetries); err != nil {
			return nil, err
		}
		t.BuildID = buildIDCol.String
		t.Name = nameCol.String
		if startedAt.Valid {
			t.StartedAt = startedAt.Time
		}
		if endedAtCol.Valid {
			t.EndedAt = endedAtCol.Time
		}
		b.Tasks = append(b.Tasks, t)
	}

	return b, nil
}

// GetAllBuilds returns all builds in the system.
func (s *Store) GetAllBuilds() ([]*model.Build, error) {
	rows, err := s.db.Query("SELECT id, status, created_at, ended_at FROM builds ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var builds []*model.Build
	for rows.Next() {
		b := &model.Build{}
		var endedAt sql.NullTime
		if err := rows.Scan(&b.ID, &b.Status, &b.CreatedAt, &endedAt); err != nil {
			return nil, err
		}
		if endedAt.Valid {
			b.EndedAt = endedAt.Time
		}
		builds = append(builds, b)
	}
	return builds, rows.Err()
}

// GetBuildGraph returns all tasks in a build and a dependency map (taskID -> list of parentIDs).
func (s *Store) GetBuildGraph(buildID string) ([]*model.Task, map[string][]string, error) {
	b, err := s.GetBuild(buildID)
	if err != nil {
		return nil, nil, err
	}
	if b == nil {
		return nil, nil, fmt.Errorf("build not found: %s", buildID)
	}

	rows, err := s.db.Query(`
		SELECT td.task_id, td.parent_id
		FROM task_dependencies td
		JOIN tasks t ON t.id = td.task_id
		WHERE t.build_id = $1`, buildID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	deps := make(map[string][]string)
	for rows.Next() {
		var taskID, parentID string
		if err := rows.Scan(&taskID, &parentID); err != nil {
			return nil, nil, err
		}
		deps[taskID] = append(deps[taskID], parentID)
	}

	return b.Tasks, deps, nil
}

// SetTasksStatus updates the status for a list of task IDs.
func (s *Store) SetTasksStatus(taskIDs []string, status model.Status) error {
	for _, id := range taskIDs {
		var endedAt sql.NullTime
		if status == model.StatusCanceled || status == model.StatusFailed || status == model.StatusSucceeded {
			endedAt = sql.NullTime{Time: time.Now(), Valid: true}
		}
		_, err := s.db.Exec("UPDATE tasks SET status = $1, ended_at = COALESCE($2, ended_at), queued_at = CASE WHEN $1 = 'QUEUED' THEN NOW() ELSE queued_at END WHERE id = $3", status, endedAt, id)
		if err != nil {
			return err
		}
	}
	return nil
}

// UpdateBuildStatus recalculates and updates the build status based on its tasks.
func (s *Store) UpdateBuildStatus(taskID string) error {
	var buildID sql.NullString
	err := s.db.QueryRow("SELECT build_id FROM tasks WHERE id = $1", taskID).Scan(&buildID)
	if err != nil || !buildID.Valid || buildID.String == "" {
		return nil
	}

	bid := buildID.String
	rows, err := s.db.Query("SELECT status, count(*) FROM tasks WHERE build_id = $1 GROUP BY status", bid)
	if err != nil {
		return err
	}
	defer rows.Close()

	counts := make(map[model.Status]int)
	total := 0
	for rows.Next() {
		var st model.Status
		var c int
		if err := rows.Scan(&st, &c); err == nil {
			counts[st] = c
			total += c
		}
	}

	var newStatus model.BuildStatus
	if counts[model.StatusRunning] > 0 {
		newStatus = model.BuildStatusRunning
	} else if counts[model.StatusQueued] > 0 || counts[model.StatusBlocked] > 0 || counts[model.StatusRetrying] > 0 {
		newStatus = model.BuildStatusPending
	} else if counts[model.StatusSucceeded] == total {
		newStatus = model.BuildStatusSucceeded
	} else {
		newStatus = model.BuildStatusFailed
	}

	var endedAt *time.Time
	if newStatus == model.BuildStatusSucceeded || newStatus == model.BuildStatusFailed {
		now := time.Now()
		endedAt = &now
	}

	_, err = s.db.Exec("UPDATE builds SET status = $1, ended_at = $2 WHERE id = $3", newStatus, endedAt, bid)
	return err
}

// ResetTables wipes the task and build tables (useful for clean testing).
func (s *Store) ResetTables() error {
	_, err := s.db.Exec("TRUNCATE builds, tasks, task_dependencies CASCADE")
	return err
}
