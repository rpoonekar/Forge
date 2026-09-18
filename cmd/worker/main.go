package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/ronavpoonekar/forge/executor"
	"github.com/ronavpoonekar/forge/proto/forgepb"
)

func main() {
	workerID := flag.String("id", "worker-1", "unique ID for this worker")
	schedulerAddr := flag.String("addr", "localhost:50051", "scheduler gRPC address")
	imageName := flag.String("image", "alpine:latest", "Docker image for task execution")
	flag.Parse()

	// 1. Create the Docker executor
	//
	// This replaces exec.Command. Instead of running commands on your Mac,
	// it creates a fresh container for each task.
	//
	// TODO (Step 2): Create the executor and pull the image
	//
	// Steps:
	//   exec, err := executor.New(*imageName)
	//   if err != nil { log.Fatalf(...) }
	//   defer exec.Close()
	//   exec.EnsureImage(context.Background())  // pulls the image if not local

	// TODO: Create executor here
	dockerExec, err := executor.New(*imageName)
	if err != nil {
		log.Fatalf("Failed to create exector: %v", err)
	}
	defer dockerExec.Close()
	if err := dockerExec.EnsureImage(context.Background()); err != nil {
		log.Fatalf("Failed to ensure image %s: %v", *imageName, err)
	}

	// 2. Connect to the scheduler
	conn, err := grpc.NewClient(*schedulerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to scheduler at %s: %v", *schedulerAddr, err)
	}
	defer conn.Close()

	client := forgepb.NewForgeServiceClient(conn)

	// 3. Register
	response, err := client.RegisterWorker(context.Background(), &forgepb.RegisterWorkerRequest{WorkerId: *workerID})
	if err != nil || !response.Ok {
		log.Fatalf("Failed to register worker: %v", err)
	}
	fmt.Printf("Worker %s connected to scheduler at %s (image: %s)\n", *workerID, *schedulerAddr, *imageName)

	// 4. Set up context for clean shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// 5. Start heartbeat goroutine
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_, err := client.Heartbeat(ctx, &forgepb.HeartbeatRequest{WorkerId: *workerID})
				if err != nil {
					log.Printf("[%s] Heartbeat failed: %v", *workerID, err)
				}
			case <-ctx.Done():
				log.Printf("[%s] Heartbeat stopped", *workerID)
				return
			}
		}
	}()

	// 6. Worker loop
	//
	// TODO (Step 3): Replace exec.Command with the Docker executor
	//
	// Old code (direct execution):
	//   cmd := exec.Command("sh", "-c", resp.Command)
	//   output, err := cmd.CombinedOutput()
	//   exitCode := cmd.ProcessState.ExitCode()
	//
	// New code (Docker execution):
	//   result, err := exec.Run(ctx, resp.Command)
	//   if err != nil { ... }
	//   output := result.Output
	//   exitCode := result.ExitCode
	//
	// Everything else in the loop stays the same — getting tasks from
	// the scheduler, reporting results, error handling.
	for {
		select {
		case <-ctx.Done():
			log.Printf("[%s] Shutting down", *workerID)
			return
		default:
		}

		resp, err := client.GetTask(ctx, &forgepb.GetTaskRequest{
			WorkerId: *workerID,
		})

		if err != nil {
			log.Printf("[%s] Failed to get task: %v", *workerID, err)
			time.Sleep(1 * time.Second)
			continue
		}
		if !resp.HasTask {
			time.Sleep(1 * time.Second)
			continue
		}

		log.Printf("[%s] Executing task %s in container: %s", *workerID, resp.TaskId, resp.Command)

		// TODO: Replace this with executor.Run()
		// result, err := exec.Run(ctx, resp.Command)
		//
		// For now, keeping the old exec.Command so the project still compiles.
		// Once you implement executor.Run() in Step 1, switch to it here.

		result, err := dockerExec.Run(ctx, resp.Command)
		if err != nil {
			log.Printf("[%s] Failed to execute task: %v", *workerID, err)

			result = &executor.Result{Output: err.Error(), ExitCode: 1}
		}

		log.Printf("[%s] Task %s finished (exit code: %d)", *workerID, resp.TaskId, result.ExitCode)

		_, err = client.ReportResult(ctx, &forgepb.ReportResultRequest{
			WorkerId: *workerID,
			TaskId:   resp.TaskId,
			Output:   result.Output,
			ExitCode: int32(result.ExitCode),
		})

		if err != nil {
			log.Printf("[%s] Failed to report result: %v", *workerID, err)
		}
	}
}
