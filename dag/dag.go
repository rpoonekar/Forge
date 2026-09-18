package dag

import (
	"errors"

	"github.com/ronavpoonekar/forge/model"
)

var (
	ErrDuplicateTaskName = errors.New("duplicate task name in build definition")
	ErrMissingDependency = errors.New("task depends on non-existent task")
	ErrCycleDetected     = errors.New("cycle detected in task dependencies")
)

// Validate checks whether a set of task specifications forms a valid
// Directed Acyclic Graph (DAG) using Kahn's algorithm and returns a valid
// topological order of task names.
//
// Returns an error if a cycle exists, if task names are duplicated,
// or if tasks reference non-existent dependencies.
func Validate(tasks []model.TaskSpec) ([]string, error) {
	names := map[string]bool{}

	for _, task := range tasks {
		if names[task.Name] {
			return nil, ErrDuplicateTaskName
		}
		names[task.Name] = true
	}

	for _, task := range tasks {
		for _, dependsOn := range task.DependsOn {
			if !names[dependsOn] {
				return nil, ErrMissingDependency
			}
		}
	}

	order := []string{}
	indegree := map[string]int{}
	adj := map[string][]string{}

	for _, task := range tasks {
		indegree[task.Name] = len(task.DependsOn)
		for _, dependency := range task.DependsOn {
			adj[dependency] = append(adj[dependency], task.Name)
		}
	}

	queue := []string{}
	for name, degree := range indegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	for len(queue) > 0 {
		front := queue[0]
		queue = queue[1:]
		order = append(order, front)

		for _, neighbor := range adj[front] {
			indegree[neighbor]--
			if indegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(order) != len(tasks) {
		return nil, ErrCycleDetected
	}

	return order, nil
}

// GetInitialRunnableTasks returns the names of all tasks that have no dependencies
// and are ready to be queued immediately upon build submission.
func GetInitialRunnableTasks(tasks []model.TaskSpec) []string {
	var ready []string
	for _, t := range tasks {
		if len(t.DependsOn) == 0 {
			ready = append(ready, t.Name)
		}
	}
	return ready
}

// FindRunnableTasks inspects the tasks in a build and determines which
// BLOCKED tasks now have all of their parent dependencies in SUCCEEDED status.
func FindRunnableTasks(tasks []*model.Task, dependencies map[string][]string) []string {
	res := []string{}

	statusMap := make(map[string]model.Status)
	for _, t := range tasks {
		statusMap[t.ID] = t.Status
	}

	for _, task := range tasks {
		if task.Status != model.StatusBlocked {
			continue
		}

		allPassed := true
		for _, dependency := range dependencies[task.ID] {
			if statusMap[dependency] != model.StatusSucceeded {
				allPassed = false
				break
			}
		}

		if allPassed {
			res = append(res, task.ID)
		}
	}

	return res
}

// FindDownstreamBlockedTasks returns all BLOCKED task IDs that directly or indirectly
// depend on a failed task, so the scheduler can cancel them.
func FindDownstreamBlockedTasks(failedTaskID string, dependencies map[string][]string, tasks []*model.Task) []string {
	// Build parent -> children map
	children := make(map[string][]string)
	for childID, parentIDs := range dependencies {
		for _, pID := range parentIDs {
			children[pID] = append(children[pID], childID)
		}
	}

	taskStatus := make(map[string]model.Status)
	for _, t := range tasks {
		taskStatus[t.ID] = t.Status
	}

	// BFS to find all reachable downstream tasks
	var toCancel []string
	visited := make(map[string]bool)
	queue := []string{failedTaskID}
	visited[failedTaskID] = true

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		for _, child := range children[curr] {
			if !visited[child] {
				visited[child] = true
				if taskStatus[child] == model.StatusBlocked {
					toCancel = append(toCancel, child)
				}
				queue = append(queue, child)
			}
		}
	}

	return toCancel
}
