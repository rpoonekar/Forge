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
// Directed Acyclic Graph (DAG) and returns a valid topological order of task names.
//
// If the graph has a cycle (e.g., A depends on B, B depends on A),
// or references unknown tasks, it must return an error.
//
// Why this is important:
// If a user submits a build with a dependency cycle, tasks in the cycle
// would wait on each other forever and deadlock the CI queue.
//
// Algorithm Guidance:
// You can use Kahn's Algorithm (BFS with In-Degrees):
//
//  1. Build an adjacency map: graph[parent] -> list of children
//     and an in-degree map: inDegree[task] -> number of dependencies it has.
//     (Be sure to check for duplicate task names and unknown dependencies!)
//
//  2. Find all tasks with inDegree == 0 (no dependencies) and push them
//     into a queue (or slice).
//
//  3. While the queue is not empty:
//     a. Pop a task `u` from the queue.
//     b. Add `u` to your topological order list.
//     c. For each child `v` of `u` (tasks that depend on `u`):
//     - Decrement inDegree[v]
//     - If inDegree[v] reaches 0, push `v` into the queue!
//
//  4. If the number of tasks in your topological order equals the total
//     number of tasks, there are NO cycles! Return the order and nil error.
//     If the count is less than total tasks, a cycle exists! Return ErrCycleDetected.
//
// TODO (Step 1): Implement Validate(tasks []model.TaskSpec) ([]string, error)
func Validate(tasks []model.TaskSpec) ([]string, error) {
	// TODO: Implement dependency validation and cycle detection using Kahn's algorithm or DFS.
	names := map[string]bool{}

	for _, task := range tasks {
		if names[task.Name] == true {
			return nil, ErrDuplicateTaskName
		}
		names[task.Name] = true
	}

	for _, task := range tasks {
		for _, dependsOn := range task.DependsOn {
			if names[dependsOn] != true {
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

// GetInitialRunnableTasks returns the names of all tasks that have NO dependencies
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
//
// This is the core runtime logic of the DAG scheduler!
//
// Parameters:
//   - tasks: slice of all tasks in the build (contains task.ID, task.Status, etc.)
//   - dependencies: map[taskID][]parentID (which tasks must complete before taskID can run)
//
// Rules for a task to be runnable:
//  1. The task's current status MUST be model.StatusBlocked.
//  2. EVERY parent task listed in dependencies[task.ID] MUST have status model.StatusSucceeded.
//
// Hint:
//  1. Build a quick lookup map of task ID -> status:
//     statusMap := make(map[string]model.Status)
//     for _, t := range tasks { statusMap[t.ID] = t.Status }
//  2. Loop through tasks:
//     if t.Status != model.StatusBlocked { continue }
//     check all parentIDs in dependencies[t.ID]:
//     if all are model.StatusSucceeded, append t.ID to your result!
//
// TODO (Step 2): Implement this function!
func FindRunnableTasks(tasks []*model.Task, dependencies map[string][]string) []string {
	// TODO: Implement this!
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
