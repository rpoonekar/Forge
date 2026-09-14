package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/exec"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/ronavpoonekar/forge/proto/forgepb"
)

func main() {
	// Stage 3: worker ID is now a CLI flag so you can run multiple workers
	//
	// Usage:
	//   go run ./cmd/worker --id worker-1
	//   go run ./cmd/worker --id worker-2
	//   go run ./cmd/worker --id worker-3
	//
	// flag.String defines a --id flag with a default value.
	// flag.Parse() reads the actual command-line arguments.

	workerID := flag.String("id", "worker-1", "unique ID for this worker")
	schedulerAddr := flag.String("addr", "localhost:50051", "scheduler gRPC address")
	flag.Parse()

	// 1. Connect to the scheduler's gRPC server
	conn, err := grpc.NewClient(*schedulerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to scheduler at %s: %v", *schedulerAddr, err)
	}
	defer conn.Close()

	client := forgepb.NewForgeServiceClient(conn)

	// 2. Register this worker with the scheduler
	//
	// TODO (Step 6): Call client.RegisterWorker() here
	//
	// This tells the scheduler "I exist, I'm ready for work."
	// Steps:
	//   1. Call client.RegisterWorker(context.Background(), &forgepb.RegisterWorkerRequest{WorkerId: *workerID})
	//   2. Check for error
	//   3. Log success
	//
	// If registration fails, the worker should exit (log.Fatalf).

	response, err := client.RegisterWorker(context.Background(), &forgepb.RegisterWorkerRequest{WorkerId: *workerID})
	if err != nil || !response.Ok {
		log.Fatalf("Failed to register worker to scheduler at %s: %v", *schedulerAddr, err)
	}

	fmt.Printf("Worker %s connected to scheduler at %s\n", *workerID, *schedulerAddr)

	// 3. Worker loop — same as Stage 2
	for {
		response, err := client.GetTask(context.Background(), &forgepb.GetTaskRequest{
			WorkerId: *workerID,
		})

		if err != nil {
			log.Printf("[%s] Failed to get task: %v", *workerID, err)
			time.Sleep(1 * time.Second)
			continue
		}
		if !response.HasTask {
			time.Sleep(1 * time.Second)
			continue
		}

		log.Printf("[%s] Executing task %s: %s", *workerID, response.TaskId, response.Command)

		cmd := exec.Command("sh", "-c", response.Command)
		output, err := cmd.CombinedOutput()

		exitCode := 0
		if err != nil {
			exitCode = 1
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
		}

		log.Printf("[%s] Task %s finished (exit code: %d)", *workerID, response.TaskId, exitCode)

		_, err = client.ReportResult(context.Background(), &forgepb.ReportResultRequest{
			WorkerId: *workerID,
			TaskId:   response.TaskId,
			Output:   string(output),
			ExitCode: int32(exitCode),
		})

		if err != nil {
			log.Printf("[%s] Failed to report result: %v", *workerID, err)
		}
	}
}
