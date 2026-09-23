package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/ronavpoonekar/forge/model"
)

// Dashboard reads one repeatable snapshot, so edges, tasks and build statuses
// cannot come from different points in a concurrent scheduler transaction.
func (s *Store) Dashboard(ctx context.Context) (*model.Snapshot, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result := &model.Snapshot{Builds: []*model.Build{}, Workers: []*model.Worker{}, Dependencies: map[string][]string{}, CapturedAt: time.Now()}
	rows, err := tx.QueryContext(ctx, `SELECT id, status, created_at, ended_at FROM builds ORDER BY created_at DESC, id LIMIT 50`)
	if err != nil {
		return nil, err
	}
	builds := map[string]*model.Build{}
	for rows.Next() {
		b := &model.Build{Tasks: []*model.Task{}}
		var end sql.NullTime
		if err := rows.Scan(&b.ID, &b.Status, &b.CreatedAt, &end); err != nil {
			rows.Close()
			return nil, err
		}
		b.EndedAt = end.Time
		builds[b.ID] = b
		result.Builds = append(result.Builds, b)
	}
	if err := finishRows(rows); err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT id, build_id, name, command, status, worker_id, created_at, started_at, ended_at, exit_code, retry_count, max_retries, next_retry_at
 FROM tasks WHERE build_id IN (SELECT id FROM builds ORDER BY created_at DESC, id LIMIT 50) ORDER BY created_at, name, id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		t := &model.Task{}
		var started, ended, retry sql.NullTime
		if err := rows.Scan(&t.ID, &t.BuildID, &t.Name, &t.Command, &t.Status, &t.WorkerID, &t.CreatedAt, &started, &ended, &t.ExitCode, &t.RetryCount, &t.MaxRetries, &retry); err != nil {
			rows.Close()
			return nil, err
		}
		t.StartedAt, t.EndedAt, t.NextRetryAt = started.Time, ended.Time, retry.Time
		builds[t.BuildID].Tasks = append(builds[t.BuildID].Tasks, t)
	}
	if err := finishRows(rows); err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT task_id, parent_id FROM task_dependencies WHERE task_id IN
 (SELECT id FROM tasks WHERE build_id IN (SELECT id FROM builds ORDER BY created_at DESC, id LIMIT 50)) ORDER BY task_id, parent_id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var task, parent string
		if err := rows.Scan(&task, &parent); err != nil {
			rows.Close()
			return nil, err
		}
		result.Dependencies[task] = append(result.Dependencies[task], parent)
	}
	if err := finishRows(rows); err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT id, status, current_task, registered_at, last_seen, tasks_run FROM workers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		w := &model.Worker{}
		if err := rows.Scan(&w.ID, &w.Status, &w.CurrentTask, &w.RegisteredAt, &w.LastSeen, &w.TasksRun); err != nil {
			rows.Close()
			return nil, err
		}
		result.Workers = append(result.Workers, w)
	}
	if err := finishRows(rows); err != nil {
		return nil, err
	}
	err = tx.QueryRowContext(ctx, `SELECT
 count(*) FILTER (WHERE status = 'QUEUED' AND (next_retry_at IS NULL OR next_retry_at <= NOW())),
 count(*) FILTER (WHERE status = 'RUNNING'),
 count(*) FILTER (WHERE status = 'SUCCEEDED' AND ended_at >= NOW() - interval '60 seconds'),
 COALESCE(sum(retry_count), 0), avg(scheduling_latency_ms) FROM tasks`).Scan(
		&result.Metrics.QueueDepth, &result.Metrics.Running, &result.Metrics.Throughput, &result.Metrics.Retries, &result.Metrics.SchedulingLatencyMS)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func finishRows(rows *sql.Rows) error {
	err := rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return err
	}
	return closeErr
}
