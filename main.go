package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/ronavpoonekar/forge/scheduler"
	"github.com/ronavpoonekar/forge/worker"
)

// Legacy in-memory single-process entry point from Stage 1.
// Production multi-worker execution uses cmd/scheduler and cmd/worker.
func main() {
	sched := scheduler.New(nil)

	w := worker.New("worker-1", sched)
	go w.Start()

	http.HandleFunc("POST /submit", func(w http.ResponseWriter, r *http.Request) {
		var data map[string]any
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		cmdVal, ok := data["command"].(string)
		if !ok || cmdVal == "" {
			http.Error(w, "Field 'command' is missing or not a string", http.StatusBadRequest)
			return
		}

		task := sched.Submit(cmdVal)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(task)
	})

	http.HandleFunc("GET /task/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		task := sched.GetTask(id)
		if task == nil {
			http.Error(w, "Task not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(task)
	})

	http.HandleFunc("GET /tasks", func(w http.ResponseWriter, r *http.Request) {
		tasks := sched.GetAllTasks()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(tasks); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}
	})

	port := 8080
	fmt.Printf("Forge is running on http://localhost:%d\n", port)
	fmt.Println("Endpoints:")
	fmt.Println("  POST /submit    — submit a task: curl -X POST -d '{\"command\":\"echo hello\"}' http://localhost:8080/submit")
	fmt.Println("  GET  /task/{id} — get task status")
	fmt.Println("  GET  /tasks     — list all tasks")
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}
