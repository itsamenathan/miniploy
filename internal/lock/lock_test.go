package lock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireExcludesConcurrentHoldersAndReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deploy.lock")
	first, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if _, err := Acquire(path); err == nil {
		t.Fatal("second Acquire() error = nil, want lock contention")
	}
	if err := first.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	second, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire() after Release error = %v", err)
	}
	defer func() { _ = second.Release() }()
}

func TestAcquireHandlesLegacyLockDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deploy.lock")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	lock, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	defer func() { _ = lock.Release() }()
}
