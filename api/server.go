// Package api serves the local dashboard, REST resources and live state stream.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ronavpoonekar/forge/dag"
	"github.com/ronavpoonekar/forge/model"
	"github.com/ronavpoonekar/forge/scheduler"
	"github.com/ronavpoonekar/forge/store"
)

type Server struct {
	sched *scheduler.Scheduler
	store *store.Store
	demo  bool
	hub   liveHub
}

func New(sched *scheduler.Scheduler, st *store.Store, demo bool) *Server {
	return &Server{sched: sched, store: st, demo: demo, hub: liveHub{clients: map[chan []byte]struct{}{}}}
}

func (s *Server) snapshot(ctx context.Context) (*model.Snapshot, error) {
	result, err := s.store.Dashboard(ctx)
	if err != nil {
		return nil, err
	}
	result.DemoEnabled = s.demo
	result.DemoWorkers, result.StoppingWorkers = s.sched.DemoWorkerState()
	return result, nil
}

func (s *Server) Handler(webDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		state, err := s.snapshot(ctx)
		if err != nil {
			serverError(w, err)
			return
		}
		respond(w, http.StatusOK, state)
	})
	mux.HandleFunc("GET /api/ws", s.websocket)
	mux.HandleFunc("POST /api/demo/builds", s.submitDemo)
	mux.HandleFunc("POST /api/demo/kill-worker", func(w http.ResponseWriter, r *http.Request) {
		if !s.demo {
			http.Error(w, "Worker failure controls are disabled. Start the scheduler with --demo-controls.", http.StatusForbidden)
			return
		}
		id, err := s.sched.KillRandomWorker()
		if errors.Is(err, scheduler.ErrNoDemoWorker) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err != nil {
			serverError(w, err)
			return
		}
		respond(w, http.StatusAccepted, map[string]string{"worker_id": id})
	})
	// Keep the original terminal API routes alongside the dashboard's /api routes.
	for _, prefix := range []string{"", "/api"} {
		mux.HandleFunc("POST "+prefix+"/submit", func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Command string `json:"command"`
			}
			if !decode(w, r, &req) {
				return
			}
			if strings.TrimSpace(req.Command) == "" {
				http.Error(w, "Command is required", 400)
				return
			}
			task := s.sched.Submit(req.Command)
			if task == nil {
				serverError(w, errors.New("submission failed"))
				return
			}
			respond(w, 201, task)
		})
		mux.HandleFunc("GET "+prefix+"/tasks", func(w http.ResponseWriter, r *http.Request) {
			tasks, err := s.store.GetAllTasks()
			if err != nil {
				serverError(w, err)
				return
			}
			if tasks == nil {
				tasks = []*model.Task{}
			}
			respond(w, 200, tasks)
		})
		mux.HandleFunc("GET "+prefix+"/task/{id}", func(w http.ResponseWriter, r *http.Request) {
			task, err := s.store.GetTask(r.PathValue("id"))
			if err != nil {
				serverError(w, err)
				return
			}
			if task == nil {
				http.NotFound(w, r)
				return
			}
			respond(w, 200, task)
		})
		mux.HandleFunc("GET "+prefix+"/workers", func(w http.ResponseWriter, r *http.Request) {
			workers, err := s.store.GetAllWorkers()
			if err != nil {
				serverError(w, err)
				return
			}
			if workers == nil {
				workers = []*model.Worker{}
			}
			respond(w, 200, workers)
		})
		mux.HandleFunc("GET "+prefix+"/builds", func(w http.ResponseWriter, r *http.Request) {
			builds, err := s.store.GetAllBuilds()
			if err != nil {
				serverError(w, err)
				return
			}
			if builds == nil {
				builds = []*model.Build{}
			}
			respond(w, 200, builds)
		})
		for _, path := range []string{"/build", "/builds"} {
			mux.HandleFunc("POST "+prefix+path, s.submitBuild)
			mux.HandleFunc("GET "+prefix+path+"/{id}", func(w http.ResponseWriter, r *http.Request) {
				b, err := s.store.GetBuild(r.PathValue("id"))
				if err != nil {
					serverError(w, err)
					return
				}
				if b == nil {
					http.NotFound(w, r)
					return
				}
				_, deps, err := s.store.GetBuildGraph(b.ID)
				if err != nil {
					serverError(w, err)
					return
				}
				respond(w, 200, struct {
					*model.Build
					Dependencies map[string][]string `json:"dependencies"`
				}{b, deps})
			})
		}
	}
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			if _, err := os.Stat(filepath.Join(webDir, "index.html")); err != nil {
				http.Error(w, "Dashboard not built. Run npm ci && npm run build in web/, then refresh.", 503)
				return
			}
		} else if !strings.HasPrefix(r.URL.Path, "/assets/") && r.URL.Path != "/favicon.svg" {
			http.NotFound(w, r)
			return
		}
		http.FileServer(http.Dir(webDir)).ServeHTTP(w, r)
	})
	return sameOrigin(mux)
}

func (s *Server) submitBuild(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tasks []model.TaskSpec `json:"tasks"`
	}
	if !decode(w, r, &req) {
		return
	}
	if len(req.Tasks) == 0 || len(req.Tasks) > 100 {
		http.Error(w, "A build must contain 1–100 tasks", 400)
		return
	}
	for _, task := range req.Tasks {
		if strings.TrimSpace(task.Name) == "" || strings.TrimSpace(task.Command) == "" {
			http.Error(w, "Each task needs a name and command", 400)
			return
		}
	}
	if _, err := dag.Validate(req.Tasks); err != nil {
		http.Error(w, "Invalid build DAG: "+err.Error(), 400)
		return
	}
	b, err := s.sched.SubmitBuild(req.Tasks)
	if err != nil {
		serverError(w, err)
		return
	}
	respond(w, 201, b)
}

func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(value); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), 400)
		return false
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		http.Error(w, "Expected one JSON object", 400)
		return false
	}
	return true
}
func respond(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("HTTP response: %v", err)
	}
}
func serverError(w http.ResponseWriter, err error) {
	log.Printf("API: %v", err)
	http.Error(w, "Unable to access cluster state. Check the scheduler log.", 500)
}

// Local execution APIs are not public endpoints. Prevent a foreign website from
// submitting workloads or stopping workers through the user's browser.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			origin := r.Header.Get("Origin")
			if origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
					http.Error(w, "Cross-origin request rejected", 403)
					return
				}
			}
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				http.Error(w, "Cross-site request rejected", 403)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
