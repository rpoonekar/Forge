package scheduler

import (
	"context"
	"log"

	"github.com/ronavpoonekar/forge/proto/forgepb"
)

// GRPCServer implements the ForgeServiceServer protobuf contract, handling
// worker registration, task polling, result reporting, and heartbeats.
type GRPCServer struct {
	forgepb.UnimplementedForgeServiceServer
	scheduler *Scheduler
}

// NewGRPCServer returns a new GRPCServer backed by the scheduler.
func NewGRPCServer(sched *Scheduler) *GRPCServer {
	return &GRPCServer{
		scheduler: sched,
	}
}

func (s *GRPCServer) RegisterWorker(ctx context.Context, req *forgepb.RegisterWorkerRequest) (*forgepb.RegisterWorkerResponse, error) {
	log.Printf("Worker %s registering", req.WorkerId)
	s.scheduler.RegisterWorker(req.WorkerId)
	return &forgepb.RegisterWorkerResponse{Ok: true}, nil
}

func (s *GRPCServer) GetTask(ctx context.Context, req *forgepb.GetTaskRequest) (*forgepb.GetTaskResponse, error) {
	task := s.scheduler.NextTask(req.WorkerId)

	if task == nil {
		return &forgepb.GetTaskResponse{HasTask: false}, nil
	}

	log.Printf("Assigned task %s to worker %s: %s", task.ID, req.WorkerId, task.Command)
	return &forgepb.GetTaskResponse{
		HasTask: true,
		TaskId:  task.ID,
		Command: task.Command,
	}, nil
}

func (s *GRPCServer) ReportResult(ctx context.Context, req *forgepb.ReportResultRequest) (*forgepb.ReportResultResponse, error) {
	log.Printf("Worker %s completed task %s (exit code: %d)", req.WorkerId, req.TaskId, req.ExitCode)
	s.scheduler.CompleteTask(req.TaskId, req.WorkerId, req.Output, int(req.ExitCode))
	return &forgepb.ReportResultResponse{Ok: true}, nil
}

// Heartbeat handles worker liveness updates to renew leases in the scheduler.
func (s *GRPCServer) Heartbeat(ctx context.Context, req *forgepb.HeartbeatRequest) (*forgepb.HeartbeatResponse, error) {
	s.scheduler.RegisterWorker(req.WorkerId)
	return &forgepb.HeartbeatResponse{Ok: true}, nil
}
