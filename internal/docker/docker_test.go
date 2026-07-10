package docker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/itsamenathan/miniploy/internal/config"
	"github.com/itsamenathan/miniploy/internal/runner"
)

func TestImageExists(t *testing.T) {
	client := testClient(t)

	t.Setenv("FAKE_DOCKER_OUTPUT", "example/app:live\n")
	exists, err := client.ImageExists(context.Background())
	if err != nil {
		t.Fatalf("ImageExists() error = %v", err)
	}
	if !exists {
		t.Fatal("ImageExists() = false, want true")
	}

	t.Setenv("FAKE_DOCKER_OUTPUT", "")
	exists, err = client.ImageExists(context.Background())
	if err != nil {
		t.Fatalf("ImageExists() error = %v", err)
	}
	if exists {
		t.Fatal("ImageExists() = true, want false")
	}
}

func testClient(t *testing.T) Client {
	t.Helper()
	dir := t.TempDir()
	docker := filepath.Join(dir, "docker")
	if err := os.WriteFile(docker, []byte("#!/bin/sh\nprintf '%s\\n' \"$FAKE_DOCKER_OUTPUT\"\n"), 0o755); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return Client{
		cfg: config.Config{ImageName: "example/app:live"},
		run: runner.Runner{},
	}
}
