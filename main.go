package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/ronavpoonekar/forge/scheduler"
	"github.com/ronavpoonekar/forge/worker"
)

func main() {
	// NOTE: This is the old Stage 1 entry point. Use cmd/scheduler/main.go instead.
	sched := scheduler.New(nil)

	// 2. Create and start a worker in a goroutine
	w := worker.New("worker-1", sched)
	go w.Start()

	// 3. Set up HTTP endpoints
	//
	// POST /submit  — submit a new task
	//   Request body: {"command": "echo hello"}
	//   Response: the created task as JSON
	//
	// GET /task/{id} — get a task's current status
	//   Response: the task as JSON
	//
	// GET /tasks     — list all tasks
	//   Response: array of all tasks as JSON
	//

	http.HandleFunc("POST /submit", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Implement this handler
		//
		// Steps:
		//   1. Decode the JSON request body to get the command
		//   2. Call sched.Submit(command)
		//   3. Return the created task as JSON
		//
		// Helpful:
		//   json.NewDecoder(r.Body).Decode(&request)
		//   json.NewEncoder(w).Encode(task)
		//   w.Header().Set("Content-Type", "application/json")
		//   w.WriteHeader(http.StatusCreated)

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
		// TODO: Implement this handler
		//
		// Steps:
		//   1. Get the task ID from the URL path: r.PathValue("id")
		//   2. Call sched.GetTask(id)
		//   3. If nil, return 404
		//   4. Otherwise, return the task as JSON
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
		// TODO: Implement this handler
		//
		// Steps:
		//   1. Call sched.GetAllTasks()
		//   2. Return the list as JSON

		tasks := sched.GetAllTasks()

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(tasks); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}
	})

	// 4. Start the HTTP server
	port := 8080
	fmt.Printf("Forge is running on http://localhost:%d\n", port)
	fmt.Println("Endpoints:")
	fmt.Println("  POST /submit    — submit a task: curl -X POST -d '{\"command\":\"echo hello\"}' http://localhost:8080/submit")
	fmt.Println("  GET  /task/{id} — get task status")
	fmt.Println("  GET  /tasks     — list all tasks")
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}
