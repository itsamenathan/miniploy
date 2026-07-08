package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/itsamenathan/miniploy/internal/config"
	"github.com/itsamenathan/miniploy/internal/deploy"
	"github.com/itsamenathan/miniploy/internal/state"
)

const readyTimeout = 10 * time.Second

type Server struct {
	cfg    config.Config
	log    *slog.Logger
	runner *deploy.Runner
	server *http.Server
	addr   string
}

type statusResponse struct {
	Status  string        `json:"status"`
	Git     gitStatus     `json:"git"`
	Compose composeStatus `json:"compose"`
	Polling pollingStatus `json:"polling"`
	State   stateSnapshot `json:"state"`
	Health  healthStatus  `json:"health"`
	Updated time.Time     `json:"updated,omitempty"`
}

type gitStatus struct {
	URL    string `json:"url"`
	Branch string `json:"branch"`
}

type composeStatus struct {
	Project string `json:"project"`
	Service string `json:"service"`
	File    string `json:"file"`
	Profile string `json:"profile,omitempty"`
}

type pollingStatus struct {
	Enabled     bool   `json:"enabled"`
	Interval    string `json:"interval"`
	DeployDelay string `json:"deployDelay"`
}

type healthStatus struct {
	Enabled bool   `json:"enabled"`
	Addr    string `json:"addr"`
}

type stateSnapshot struct {
	LastDeployedCommit  string    `json:"lastDeployedCommit,omitempty"`
	LastSuccessfulImage string    `json:"lastSuccessfulImage,omitempty"`
	LastAttemptedCommit string    `json:"lastAttemptedCommit,omitempty"`
	LastComposeHash     string    `json:"lastComposeHash,omitempty"`
	LastStatus          string    `json:"lastStatus,omitempty"`
	LastError           string    `json:"lastError,omitempty"`
	LastErrorAt         time.Time `json:"lastErrorAt,omitempty"`
	Updated             time.Time `json:"updated,omitempty"`
}

type readyResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
	Error  string            `json:"error,omitempty"`
}

func New(cfg config.Config, log *slog.Logger) *Server {
	return &Server{cfg: cfg, log: log, runner: deploy.New(cfg, log)}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("GET /status", s.status)

	s.server = &http.Server{
		Addr:              s.cfg.HealthAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	listener, err := net.Listen("tcp", s.cfg.HealthAddr)
	if err != nil {
		return err
	}
	go func() {
		if err := s.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) && s.log != nil {
			s.log.Error("health server failed", "error", err)
		}
	}()
	s.addr = listener.Addr().String()
	if s.log != nil {
		s.log.Info("health server started", "addr", s.addr)
	}
	return nil
}

func (s *Server) Addr() string {
	return s.addr
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
	defer cancel()

	checks := map[string]string{"state": "ok"}
	if err := s.checkStateWritable(); err != nil {
		checks["state"] = "error"
		writeJSON(w, http.StatusServiceUnavailable, readyResponse{Status: "error", Checks: checks, Error: err.Error()})
		return
	}

	if err := s.runner.Validate(ctx); err != nil {
		checks["docker_compose"] = "error"
		writeJSON(w, http.StatusServiceUnavailable, readyResponse{Status: "error", Checks: checks, Error: err.Error()})
		return
	}
	checks["docker_compose"] = "ok"
	writeJSON(w, http.StatusOK, readyResponse{Status: "ok", Checks: checks})
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	st, err := state.Load(s.cfg.StatePath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": fmt.Sprintf("load state: %v", err)})
		return
	}

	lastStatus := st.LastStatus
	if lastStatus == "" {
		lastStatus = "unknown"
	}
	writeJSON(w, http.StatusOK, statusResponse{
		Status: lastStatus,
		Git: gitStatus{
			URL:    s.cfg.GitURL,
			Branch: s.cfg.GitBranch,
		},
		Compose: composeStatus{
			Project: s.cfg.ComposeProjectName,
			Service: s.cfg.ComposeService,
			File:    s.cfg.ComposeFile,
			Profile: s.cfg.ComposeProfile,
		},
		Polling: pollingStatus{
			Enabled:     s.cfg.CheckInterval > 0,
			Interval:    s.cfg.CheckInterval.String(),
			DeployDelay: s.cfg.DeployDelay.String(),
		},
		State: stateSnapshot{
			LastDeployedCommit:  st.LastDeployedCommit,
			LastSuccessfulImage: st.LastSuccessfulImage,
			LastAttemptedCommit: st.LastAttemptedCommit,
			LastComposeHash:     st.LastComposeHash,
			LastStatus:          st.LastStatus,
			LastError:           st.LastError,
			LastErrorAt:         st.LastErrorAt,
			Updated:             st.Updated,
		},
		Health: healthStatus{
			Enabled: s.cfg.HealthEnabled,
			Addr:    s.cfg.HealthAddr,
		},
		Updated: time.Now().UTC(),
	})
}

func (s *Server) checkStateWritable() error {
	dir := filepath.Dir(s.cfg.StatePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".health-*")
	if err != nil {
		return err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return os.Remove(path)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
