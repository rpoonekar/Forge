package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/ronavpoonekar/forge/proto/forgepb"
)

func main() {
	workerID := flag.String("id", "worker-1", "unique ID for this worker")
	schedulerAddr := flag.String("addr", "localhost:50051", "scheduler gRPC address")
	flag.Parse()

	// 1. Connect to the scheduler
	conn, err := grpc.NewClient(*schedulerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to scheduler at %s: %v", *schedulerAddr, err)
	}
	defer conn.Close()

	client := forgepb.NewForgeServiceClient(conn)

	// 2. Register
	response, err := client.RegisterWorker(context.Background(), &forgepb.RegisterWorkerRequest{WorkerId: *workerID})
	if err != nil || !response.Ok {
		log.Fatalf("Failed to register worker: %v", err)
	}
	fmt.Printf("Worker %s connected to scheduler at %s\n", *workerID, *schedulerAddr)

	// 3. Set up context for clean shutdown
	//
	// signal.NotifyContext creates a context that automatically cancels when
	// the process receives SIGINT (Ctrl+C) or SIGTERM (kill command).
	//
	// When ctx is cancelled:
	//   - The heartbeat goroutine stops (it checks <-ctx.Done())
	//   - The main loop exits (it checks <-ctx.Done())
	//
	// This is how Go programs handle graceful shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// 4. Start heartbeat goroutine
	//
	// This runs in the background, sending a Heartbeat RPC every 5 seconds.
	// It stops when ctx is cancelled (Ctrl+C or process death).
	//
	// TODO (Step 5): Implement the heartbeat goroutine
	//
	// go func() {
	//     ticker := time.NewTicker(5 * time.Second)
	//     defer ticker.Stop()
	//
	//     for {
	//         select {
	//         case <-ticker.C:
	//             _, err := client.Heartbeat(ctx, &forgepb.HeartbeatRequest{WorkerId: *workerID})
	//             if err != nil {
	//                 log.Printf("[%s] Heartbeat failed: %v", *workerID, err)
	//             }
	//         case <-ctx.Done():
	//             log.Printf("[%s] Heartbeat stopped", *workerID)
	//             return
	//         }
	//     }
	// }()

	// TODO: Start the heartbeat goroutine here (use the skeleton above)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <- ticker.C:
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

	// 5. Worker loop — same as before, but now checks ctx for shutdown
	for {
		// Check if we should shut down
		select {
		case <-ctx.Done():
			log.Printf("[%s] Shutting down", *workerID)
			return
		default:
			// continue working
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

		log.Printf("[%s] Executing task %s: %s", *workerID, resp.TaskId, resp.Command)

		cmd := exec.Command("sh", "-c", resp.Command)
		output, err := cmd.CombinedOutput()

		exitCode := 0
		if err != nil {
			exitCode = 1
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
		}

		log.Printf("[%s] Task %s finished (exit code: %d)", *workerID, resp.TaskId, exitCode)

		_, err = client.ReportResult(ctx, &forgepb.ReportResultRequest{
			WorkerId: *workerID,
			TaskId:   resp.TaskId,
			Output:   string(output),
			ExitCode: int32(exitCode),
		})

		if err != nil {
			log.Printf("[%s] Failed to report result: %v", *workerID, err)
		}
	}
}
