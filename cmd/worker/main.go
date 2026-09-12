package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/ronavpoonekar/forge/proto/forgepb"
)

func main() {
	// The worker is now a SEPARATE binary that connects to the scheduler over gRPC.
	//
	// Compare this to Stage 1's worker:
	//   Stage 1: w.scheduler.NextTask()       — direct function call, same process
	//   Stage 2: client.GetTask(ctx, &req)     — gRPC call, over the network
	//
	// The worker loop logic (get task → execute → report) is the SAME.
	// Only HOW it talks to the scheduler changes.

	workerID := "worker-1" // TODO: you could make this a CLI flag later

	// 1. Connect to the scheduler's gRPC server
	schedulerAddr := "localhost:50051"
	conn, err := grpc.NewClient(schedulerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to scheduler at %s: %v", schedulerAddr, err)
	}
	defer conn.Close()

	// Create a gRPC client from the connection
	client := forgepb.NewForgeServiceClient(conn)

	fmt.Printf("Worker %s connected to scheduler at %s\n", workerID, schedulerAddr)

	// 2. Worker loop — same logic as Stage 1, but over gRPC
	//
	// TODO (Step 5): Implement the worker loop
	//
	// The loop should:
	//   a) Call client.GetTask() with a GetTaskRequest containing the worker ID
	//   b) If response.HasTask is false, sleep and retry
	//   c) If response.HasTask is true:
	//      - Execute the command with exec.Command("sh", "-c", response.Command)
	//      - Capture output and exit code (same as Stage 1)
	//      - Call client.ReportResult() with the task ID, output, and exit code
	//   d) Repeat forever
	//
	// For gRPC calls, you need a context:
	//   ctx := context.Background()
	//   resp, err := client.GetTask(ctx, &forgepb.GetTaskRequest{WorkerId: workerID})
	//
	// Hint: your Stage 1 worker.Start() had this exact logic — just swap
	// the direct scheduler calls for gRPC client calls.

	for {
		response, err := client.GetTask(context.Background(), &forgepb.GetTaskRequest{
			WorkerId: workerID,
		})

		if err != nil {
			log.Printf("Failed to get task: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		if response.HasTask == false {
			time.Sleep(1 * time.Second)
			continue
		}

		cmd := exec.Command("sh", "-c", response.Command)
		output, err := cmd.CombinedOutput()
		
		exitCode := 0
		if err != nil {
			exitCode = 1
			// If process actually ran, get the real exit code
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
		}

		result := &forgepb.ReportResultRequest{
			WorkerId: workerID,
			TaskId: response.TaskId,
			Output: string(output),
			ExitCode: int32(exitCode),
		}

		client.ReportResult(context.Background(), result)

		if err != nil {
			log.Printf("Failed to report result: %v", err)
		}
	}

}
