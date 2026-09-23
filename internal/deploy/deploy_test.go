package deploy

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/itsamenathan/miniploy/internal/config"
	"github.com/itsamenathan/miniploy/internal/pause"
	"github.com/itsamenathan/miniploy/internal/state"
)

func TestRecordFailureReturnsPersistenceError(t *testing.T) {
	failure := errors.New("build failed")
	persistenceErr := errors.New("disk full")
	runner := Runner{stateSaver: func(state.State) error { return persistenceErr }}

	err := runner.recordFailure(state.State{}, "abc123", failure)
	if !errors.Is(err, failure) {
		t.Fatalf("recordFailure() error = %v, want original failure", err)
	}
	if !errors.Is(err, persistenceErr) {
		t.Fatalf("recordFailure() error = %v, want persistence failure", err)
	}
}

func TestGitCheckFailureDoesNotLeaveOverallStatusSuccessful(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{
		DataDir:       dir,
		StatePath:     filepath.Join(dir, "state.json"),
		LockDir:       filepath.Join(dir, "deploy.lock"),
		GitAuthMode:   "ssh",
		GitSSHKeyPath: filepath.Join(dir, "missing-key"),
	}
	if err := state.Save(cfg.StatePath, state.State{LastStatus: "success", LastDeployedCommit: "abc123"}); err != nil {
		t.Fatal(err)
	}
	runner := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := runner.RunOnce(context.Background(), "poll"); err == nil {
		t.Fatal("RunOnce() succeeded with missing SSH key")
	}
	st, err := state.Load(cfg.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastStatus != "success" || st.LastCheckStatus != "failed" || st.EffectiveStatus() != "failed" || st.LastCheckAt.IsZero() {
		t.Fatalf("state after failed check = %+v", st)
	}
}

func TestPausedRunSkipsAutomaticCheck(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir, StatePath: filepath.Join(dir, "state.json"), LockDir: filepath.Join(dir, "deploy.lock")}
	if err := pause.Set(dir, true); err != nil {
		t.Fatal(err)
	}
	runner := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := runner.RunOnce(context.Background(), "poll"); err != nil {
		t.Fatalf("RunOnce() while paused: %v", err)
	}
	if _, err := os.Stat(cfg.StatePath); !os.IsNotExist(err) {
		t.Fatalf("paused check wrote deployment state: %v", err)
	}
}
