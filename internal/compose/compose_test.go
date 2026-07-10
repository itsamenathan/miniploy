package compose

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/itsamenathan/miniploy/internal/config"
	"github.com/itsamenathan/miniploy/internal/runner"
)

func TestHashIsStableAndSensitiveToRenderedConfig(t *testing.T) {
	first := "services:\n  app:\n    image: app:one\n"
	if got := Hash(first); got != Hash(first) {
		t.Fatalf("Hash() = %q on identical input, want stable value", got)
	}
	if Hash(first) == Hash("services:\n  app:\n    image: app:two\n") {
		t.Fatal("Hash() is identical for different rendered configurations")
	}
}

func TestRunning(t *testing.T) {
	client := testClient(t)

	t.Setenv("FAKE_DOCKER_OUTPUT", "app\n")
	running, err := client.Running(context.Background())
	if err != nil {
		t.Fatalf("Running() error = %v", err)
	}
	if !running {
		t.Fatal("Running() = false, want true")
	}

	t.Setenv("FAKE_DOCKER_OUTPUT", "")
	running, err = client.Running(context.Background())
	if err != nil {
		t.Fatalf("Running() error = %v", err)
	}
	if running {
		t.Fatal("Running() = true, want false")
	}
}

func TestUpVerifiesServiceIsRunning(t *testing.T) {
	client := testClient(t)
	t.Setenv("FAKE_DOCKER_OUTPUT", "app\n")

	if err := client.Up(context.Background()); err != nil {
		t.Fatalf("Up() error = %v, want nil", err)
	}
}

func TestUpFailsWhenServiceIsNotRunning(t *testing.T) {
	client := testClient(t)
	t.Setenv("FAKE_DOCKER_OUTPUT", "")

	err := client.Up(context.Background())
	if err == nil {
		t.Fatal("Up() error = nil, want service verification error")
	}
}

func testClient(t *testing.T) Client {
	t.Helper()
	dir := t.TempDir()
	docker := filepath.Join(dir, "docker")
	script := `#!/bin/sh
case "$*" in
  *"ps --status running --services app"*) printf '%s\n' "$FAKE_DOCKER_OUTPUT" ;;
esac
`
	if err := os.WriteFile(docker, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return Client{
		cfg: config.Config{
			ComposeFile:        "/compose/compose.yaml",
			ComposeProjectName: "test-project",
			ComposeService:     "app",
			RedeployArgs:       []string{"--no-deps", "--force-recreate"},
		},
		run: runner.Runner{},
	}
}
