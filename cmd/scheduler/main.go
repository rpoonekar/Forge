package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"

	"google.golang.org/grpc"

	"github.com/ronavpoonekar/forge/proto/forgepb"
	"github.com/ronavpoonekar/forge/scheduler"
)

func main() {
	// 1. Create the scheduler (same as Stage 1 — this doesn't change)
	sched := scheduler.New()

	// 2. Start the gRPC server so workers can connect
	//
	// This replaces the old "go w.Start()" — instead of running a worker
	// in-process, we listen for workers to connect over the network.
	//
	// A gRPC server needs:
	//   a) A TCP listener (what port to listen on)
	//   b) A gRPC server instance
	//   c) Your service implementation registered on that server
	//
	grpcServer := grpc.NewServer()

	// TODO (Step 3): Create your GRPCServer and register it
	//
	// grpcHandler := scheduler.NewGRPCServer(sched)
	// forgepb.RegisterForgeServiceServer(grpcServer, grpcHandler)
	grpcHandler := scheduler.NewGRPCServer(sched)
	forgepb.RegisterForgeServiceServer(grpcServer, grpcHandler)

	grpcPort := 50051
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		log.Fatalf("Failed to listen on port %d: %v", grpcPort, err)
	}

	// Run gRPC server in a goroutine (it blocks, and we still need HTTP below)
	go func() {
		fmt.Printf("gRPC server listening on :%d\n", grpcPort)
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// 3. HTTP endpoints (same as Stage 1 — carry over your handlers)
	//
	// These stay on the scheduler because the scheduler is the "brain"
	// that knows about all tasks. The HTTP API is how users interact.

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
		json.NewEncoder(w).Encode(task)
	})

	http.HandleFunc("GET /tasks", func(w http.ResponseWriter, r *http.Request) {
		tasks := sched.GetAllTasks()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tasks)
	})

	// Stage 3: new endpoint to see registered workers
	http.HandleFunc("GET /workers", func(w http.ResponseWriter, r *http.Request) {
		workers := sched.GetAllWorkers()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(workers)
	})

	// 4. Start the HTTP server
	httpPort := 8080
	fmt.Printf("HTTP server listening on :%d\n", httpPort)
	fmt.Println("Endpoints:")
	fmt.Println("  POST /submit    — submit a task")
	fmt.Println("  GET  /task/{id} — get task status")
	fmt.Println("  GET  /tasks     — list all tasks")
	fmt.Println("  GET  /workers   — list all workers")
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", httpPort), nil))
}
