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
	demoControl := flag.Bool("demo-control", false, "allow the dashboard to interrupt this worker for a failure demo")
	flag.Parse()

	// 1. Create the Docker executor
	dockerExec, err := executor.New(*imageName)
	if err != nil {
		log.Fatalf("Failed to create executor: %v", err)
	}
	defer dockerExec.Close()
	if err := dockerExec.EnsureImage(context.Background()); err != nil {
		log.Fatalf("Failed to ensure image %s: %v", *imageName, err)
	}

	// 2. Connect to the scheduler via gRPC
	conn, err := grpc.NewClient(*schedulerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to scheduler at %s: %v", *schedulerAddr, err)
	}
	defer conn.Close()

	client := forgepb.NewForgeServiceClient(conn)

	// 3. Register worker with scheduler
	response, err := client.RegisterWorker(context.Background(), &forgepb.RegisterWorkerRequest{WorkerId: *workerID, DemoControl: *demoControl})
	if err != nil || !response.Ok {
		log.Fatalf("Failed to register worker: %v", err)
	}
	fmt.Printf("Worker %s connected to scheduler at %s (image: %s)\n", *workerID, *schedulerAddr, *imageName)

	// 4. Set up context for clean shutdown (SIGINT / SIGTERM)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// 5. Start background heartbeat loop (renews lease every 5s)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				reply, err := client.Heartbeat(ctx, &forgepb.HeartbeatRequest{WorkerId: *workerID})
				if err != nil {
					log.Printf("[%s] Heartbeat failed: %v", *workerID, err)
				} else if reply.StopWorker && *demoControl {
					log.Printf("[%s] Dashboard requested failure injection; stopping without reporting a result", *workerID)
					cancel()
					return
				}
			case <-ctx.Done():
				log.Printf("[%s] Heartbeat stopped", *workerID)
				return
			}
		}
	}()

	// 6. Worker polling and execution loop
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

		result, err := dockerExec.Run(ctx, resp.Command)
		if ctx.Err() != nil {
			return
		}
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
