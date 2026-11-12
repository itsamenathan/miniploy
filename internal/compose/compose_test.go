package compose

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestValidateChecksManagedImage(t *testing.T) {
	client := testClient(t)
	client.cfg.ComposeFile = filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(client.cfg.ComposeFile, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("FAKE_DOCKER_CONFIG", `{"services":{"app":{"image":"other:live"}}}`)
	err := client.Validate(context.Background())
	if err == nil || !strings.Contains(err.Error(), "IMAGE_NAME must match") {
		t.Fatalf("Validate() error = %v, want image mismatch", err)
	}

	t.Setenv("FAKE_DOCKER_CONFIG", `{"services":{"app":{"image":"app:live"}}}`)
	if err := client.Validate(context.Background()); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidateAcceptsEquivalentImageReferences(t *testing.T) {
	client := testClient(t)
	client.cfg.ComposeFile = filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(client.cfg.ComposeFile, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ composeImage, imageName string }{
		{"docker.io/library/app:live", "app:live"},
		{"app", "app:latest"},
		{"docker.io/org/app", "org/app:latest"},
	}
	for _, tc := range cases {
		client.cfg.ImageName = tc.imageName
		t.Setenv("FAKE_DOCKER_CONFIG", `{"services":{"app":{"image":"`+tc.composeImage+`"}}}`)
		if err := client.Validate(context.Background()); err != nil {
			t.Fatalf("Validate(%q vs %q) error = %v", tc.composeImage, tc.imageName, err)
		}
	}
}

func TestNormalizeImage(t *testing.T) {
	cases := map[string]string{
		"app":                              "app:latest",
		"app:live":                         "app:live",
		"docker.io/library/app:live":       "app:live",
		"registry.example.com:5000/app":    "registry.example.com:5000/app:latest",
		"ghcr.io/org/app:v1":               "ghcr.io/org/app:v1",
		"app@sha256:abc":                   "app@sha256:abc",
		"registry.example.com/library/app": "registry.example.com/library/app:latest",
	}
	for in, want := range cases {
		if got := NormalizeImage(in); got != want {
			t.Errorf("NormalizeImage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUpWaitsForAppHealth(t *testing.T) {
	client := testClient(t)
	client.cfg.DeployWaitTimeout = 60 * time.Second
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("FAKE_DOCKER_ARGS_FILE", argsFile)
	t.Setenv("FAKE_DOCKER_OUTPUT", "app\n")
	if err := client.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "up -d --no-deps --force-recreate --wait --wait-timeout 60 app") {
		t.Fatalf("up args = %q, want health wait", args)
	}
}

func testClient(t *testing.T) Client {
	t.Helper()
	dir := t.TempDir()
	docker := filepath.Join(dir, "docker")
	script := `#!/bin/sh
case "$*" in
  *"config --format json"*) printf '%s\n' "$FAKE_DOCKER_CONFIG" ;;
  *"up -d"*) if [ -n "$FAKE_DOCKER_ARGS_FILE" ]; then printf '%s\n' "$*" > "$FAKE_DOCKER_ARGS_FILE"; fi ;;
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
			ImageName:          "app:live",
			RedeployArgs:       []string{"--no-deps", "--force-recreate"},
			DeployWaitTimeout:  60 * time.Second,
		},
		run: runner.Runner{},
	}
}
