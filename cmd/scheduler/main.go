package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/ronavpoonekar/forge/model"
	"github.com/ronavpoonekar/forge/proto/forgepb"
	"github.com/ronavpoonekar/forge/scheduler"
	"github.com/ronavpoonekar/forge/store"
)

func main() {
	// 1. Connect to PostgreSQL and create the store
	dbConnStr := "postgres://localhost:5432/forge?sslmode=disable"
	st, err := store.New(dbConnStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer st.Close()
	fmt.Println("Connected to PostgreSQL")

	// 2. Create the scheduler backed by the database store
	sched := scheduler.New(st)

	// Stage 5: Create a context for graceful shutdown and start the lease checker
	//
	// signal.NotifyContext creates a context that cancels automatically on Ctrl+C.
	// We pass this context to the lease checker so it stops cleanly on shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Exit the process cleanly when Ctrl+C cancels the context
	go func() {
		<-ctx.Done()
		os.Exit(0)
	}()

	// Start the lease checker in the background
	// Checks every 10 seconds for workers that haven't heartbeated in 15 seconds
	go sched.StartLeaseChecker(ctx, 10*time.Second, 15*time.Second)

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

	// Stage 8: Build endpoints for DAG workflows
	submitBuildHandler := func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Tasks []model.TaskSpec `json:"tasks"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if len(req.Tasks) == 0 {
			http.Error(w, "Field 'tasks' cannot be empty", http.StatusBadRequest)
			return
		}

		build, err := sched.SubmitBuild(req.Tasks)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid build DAG: %v", err), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(build)
	}
	http.HandleFunc("POST /build", submitBuildHandler)
	http.HandleFunc("POST /builds", submitBuildHandler)

	getBuildHandler := func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		build, err := sched.GetBuild(id)
		if err != nil {
			http.Error(w, "Failed to retrieve build: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if build == nil {
			http.Error(w, "Build not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(build)
	}
	http.HandleFunc("GET /build/{id}", getBuildHandler)
	http.HandleFunc("GET /builds/{id}", getBuildHandler)

	http.HandleFunc("GET /builds", func(w http.ResponseWriter, r *http.Request) {
		builds := sched.GetAllBuilds()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(builds)
	})

	// 4. Start the HTTP server
	httpPort := 8080
	fmt.Printf("HTTP server listening on :%d\n", httpPort)
	fmt.Println("Endpoints:")
	fmt.Println("  POST /submit    — submit a single task")
	fmt.Println("  POST /build     — submit a DAG build with dependencies")
	fmt.Println("  GET  /build/{id}— get build status and all task states")
	fmt.Println("  GET  /task/{id} — get task status")
	fmt.Println("  GET  /tasks     — list all tasks")
	fmt.Println("  GET  /workers   — list all workers")
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", httpPort), nil))
}
