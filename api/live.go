package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// liveHub fans out complete snapshots. Each subscriber retains only the newest
// snapshot; a slow browser cannot block another browser or the scheduler.
type liveHub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
	latest  []byte
}

func (h *liveHub) publish(data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.latest = data
	for ch := range h.clients {
		select {
		case <-ch:
		default:
		}
		ch <- data
	}
}

func (h *liveHub) subscribe() (chan []byte, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan []byte, 1)
	h.clients[ch] = struct{}{}
	if h.latest != nil {
		ch <- h.latest
	}
	return ch, func() { h.mu.Lock(); delete(h.clients, ch); h.mu.Unlock() }
}

// Run coalesces scheduler notifications into at most ten snapshots per second.
// The periodic refresh advances time-window metrics and recovers from DB outages.
func (s *Server) Run(ctx context.Context) {
	refresh := func() {
		readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		snapshot, err := s.snapshot(readCtx)
		var payload any = map[string]any{"type": "snapshot", "data": snapshot}
		if err != nil {
			log.Printf("Dashboard snapshot: %v", err)
			payload = map[string]string{"type": "error", "message": "Unable to read cluster state. Retrying…"}
		}
		data, err := json.Marshal(payload)
		if err == nil {
			s.hub.publish(data)
		}
	}
	refresh()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	periodic := time.NewTicker(5 * time.Second)
	defer periodic.Stop()
	dirty := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.sched.Changes():
			dirty = true
		case <-ticker.C:
			if dirty {
				refresh()
				dirty = false
			}
		case <-periodic.C:
			refresh()
			dirty = false
		}
	}
}

var upgrader = websocket.Upgrader{HandshakeTimeout: 5 * time.Second}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	ch, unsubscribe := s.hub.subscribe()
	defer unsubscribe()
	conn.SetReadLimit(1024)
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(60 * time.Second)) })
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-done:
			return
		case data := <-ch:
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ping.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}
}
