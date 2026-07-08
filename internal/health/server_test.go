package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/itsamenathan/miniploy/internal/config"
	"github.com/itsamenathan/miniploy/internal/state"
)

func TestServerStartAndShutdown(t *testing.T) {
	cfg := testConfig(t)
	cfg.HealthAddr = "127.0.0.1:0"
	server := New(cfg, nil)

	if err := server.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if server.Addr() == "" {
		t.Fatal("Addr() is empty after Start()")
	}
	resp, err := http.Get("http://" + server.Addr() + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestServerMethodNotAllowed(t *testing.T) {
	cfg := testConfig(t)
	cfg.HealthAddr = "127.0.0.1:0"
	server := New(cfg, nil)
	if err := server.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	resp, err := http.Post("http://"+server.Addr()+"/healthz", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /healthz: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestHealthz(t *testing.T) {
	server := New(testConfig(t), nil)
	recorder := httptest.NewRecorder()

	server.healthz(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["status"] != "ok" {
		t.Fatalf("status body = %q, want ok", response["status"])
	}
}

func TestReadyzReportsValidationFailure(t *testing.T) {
	server := New(testConfig(t), nil)
	recorder := httptest.NewRecorder()

	server.readyz(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	var response readyResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != "error" {
		t.Fatalf("Status = %q, want error", response.Status)
	}
	if response.Checks["state"] != "ok" {
		t.Fatalf("state check = %q, want ok", response.Checks["state"])
	}
	if response.Checks["docker_compose"] != "error" {
		t.Fatalf("docker_compose check = %q, want error", response.Checks["docker_compose"])
	}
}

func TestStatus(t *testing.T) {
	cfg := testConfig(t)
	st := state.State{
		LastDeployedCommit:  "abc123",
		LastSuccessfulImage: "app:abc123",
		LastAttemptedCommit: "abc123",
		LastComposeHash:     "deadbeef",
		LastStatus:          "success",
	}
	if err := state.Save(cfg.StatePath, st); err != nil {
		t.Fatalf("save state: %v", err)
	}

	server := New(cfg, nil)
	recorder := httptest.NewRecorder()
	server.status(recorder, httptest.NewRequest(http.MethodGet, "/status", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response statusResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != "success" {
		t.Fatalf("Status = %q, want success", response.Status)
	}
	if response.Git.Branch != cfg.GitBranch {
		t.Fatalf("Git.Branch = %q, want %q", response.Git.Branch, cfg.GitBranch)
	}
	if response.Compose.Service != cfg.ComposeService {
		t.Fatalf("Compose.Service = %q, want %q", response.Compose.Service, cfg.ComposeService)
	}
	if !response.Polling.Enabled {
		t.Fatal("Polling.Enabled = false, want true")
	}
	if response.State.LastDeployedCommit != st.LastDeployedCommit {
		t.Fatalf("State.LastDeployedCommit = %q, want %q", response.State.LastDeployedCommit, st.LastDeployedCommit)
	}
}

func TestStatusDefaultsToUnknown(t *testing.T) {
	server := New(testConfig(t), nil)
	recorder := httptest.NewRecorder()

	server.status(recorder, httptest.NewRequest(http.MethodGet, "/status", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response statusResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != "unknown" {
		t.Fatalf("Status = %q, want unknown", response.Status)
	}
}

func TestStatusReportsPollingDisabled(t *testing.T) {
	cfg := testConfig(t)
	cfg.CheckInterval = 0
	server := New(cfg, nil)
	recorder := httptest.NewRecorder()

	server.status(recorder, httptest.NewRequest(http.MethodGet, "/status", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response statusResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Polling.Enabled {
		t.Fatal("Polling.Enabled = true, want false")
	}
}

func TestWriteJSONContentType(t *testing.T) {
	recorder := httptest.NewRecorder()

	writeJSON(recorder, http.StatusAccepted, map[string]string{"status": "ok"})

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusAccepted)
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
}

func TestCheckStateWritable(t *testing.T) {
	server := New(testConfig(t), nil)

	if err := server.checkStateWritable(); err != nil {
		t.Fatalf("checkStateWritable() error = %v", err)
	}
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	dir := t.TempDir()
	return config.Config{
		GitURL:             "https://example.com/repo.git",
		GitBranch:          "main",
		ImageName:          "app:live",
		ComposeFile:        filepath.Join(dir, "compose.yaml"),
		ComposeProjectName: "app",
		ComposeService:     "web",
		HealthEnabled:      true,
		HealthAddr:         "127.0.0.1:8080",
		CheckInterval:      5 * time.Minute,
		StatePath:          filepath.Join(dir, "state.json"),
		DataDir:            dir,
		LockDir:            filepath.Join(dir, "deploy.lock"),
	}
}
