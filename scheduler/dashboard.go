package scheduler

import (
	"errors"
	"math/rand/v2"
	"time"

	"github.com/ronavpoonekar/forge/model"
)

// Changes is a coalesced invalidation stream: consumers read fresh durable state.
// A slow dashboard must never hold up task execution.
func (s *Scheduler) Changes() <-chan struct{} { return s.changes }
func (s *Scheduler) Notify() {
	select {
	case s.changes <- struct{}{}:
	default:
	}
}

var ErrNoDemoWorker = errors.New("no responsive, opted-in workers available")

// KillRandomWorker asks an opted-in worker to stop at its next heartbeat. It does
// not mark the worker offline or requeue work: the real lease checker does that.
// Prefer busy workers so the demo exercises recovery of an interrupted task.
func (s *Scheduler) KillRandomWorker() (string, error) {
	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	workers, err := s.store.GetAllWorkers()
	if err != nil {
		return "", err
	}
	var candidates, busy []string
	for _, w := range workers {
		if !s.demoWorkers[w.ID] || s.stopping[w.ID] || w.Status == model.WorkerStatusOffline || time.Since(w.LastSeen) > 15*time.Second {
			continue
		}
		candidates = append(candidates, w.ID)
		if w.Status == model.WorkerStatusBusy {
			busy = append(busy, w.ID)
		}
	}
	if len(busy) > 0 {
		candidates = busy
	}
	if len(candidates) == 0 {
		return "", ErrNoDemoWorker
	}
	id := candidates[rand.IntN(len(candidates))]
	s.stopping[id] = true
	s.Notify()
	return id, nil
}

func (s *Scheduler) DemoWorkerState() (eligible, stopping []string) {
	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	eligible, stopping = []string{}, []string{}
	for id, enabled := range s.demoWorkers {
		if enabled {
			eligible = append(eligible, id)
		}
	}
	for id := range s.stopping {
		stopping = append(stopping, id)
	}
	return
}
