package api

import (
	"fmt"
	"net/http"

	"github.com/ronavpoonekar/forge/model"
)

// Demo workloads are fixed on the server and only run harmless container commands.
func demoTasks(kind string) ([]model.TaskSpec, bool) {
	switch kind {
	case "pipeline", "failure":
		test := "echo 'Running tests'; sleep 20; echo 'All tests passed'"
		if kind == "failure" {
			test = "echo 'Running tests'; sleep 8; echo 'Demo assertion failed'; exit 1"
		}
		return []model.TaskSpec{
			{Name: "checkout", Command: "echo 'Preparing workspace'; sleep 3; echo 'Workspace ready'"},
			{Name: "lint", Command: "echo 'Checking formatting'; sleep 15; echo 'Lint passed'", DependsOn: []string{"checkout"}},
			{Name: "test", Command: test, DependsOn: []string{"checkout"}},
			{Name: "compile", Command: "echo 'Compiling'; sleep 6; echo 'Build complete'", DependsOn: []string{"lint", "test"}},
			{Name: "package", Command: "echo 'Packaging'; sleep 3; echo 'Artifact ready'", DependsOn: []string{"compile"}},
		}, true
	case "parallel":
		tasks := make([]model.TaskSpec, 20)
		for i := range tasks {
			tasks[i] = model.TaskSpec{Name: fmt.Sprintf("task-%02d", i+1), Command: "echo 'Starting parallel task'; sleep 10; echo 'Done'"}
		}
		return tasks, true
	default:
		return nil, false
	}
}

func (s *Server) submitDemo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind string `json:"kind"`
	}
	if !decode(w, r, &req) {
		return
	}
	tasks, ok := demoTasks(req.Kind)
	if !ok {
		http.Error(w, "Unknown workload", 400)
		return
	}
	b, err := s.sched.SubmitBuild(tasks)
	if err != nil {
		serverError(w, err)
		return
	}
	respond(w, http.StatusCreated, b)
}
