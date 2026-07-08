package config

import (
	"testing"
	"time"
)

func TestLoadCheckIntervalDefault(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CheckInterval != 5*time.Minute {
		t.Fatalf("CheckInterval = %v, want %v", cfg.CheckInterval, 5*time.Minute)
	}
}

func TestLoadCheckIntervalSeconds(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CHECK_INTERVAL", "30")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CheckInterval != 30*time.Second {
		t.Fatalf("CheckInterval = %v, want %v", cfg.CheckInterval, 30*time.Second)
	}
}

func TestLoadCheckIntervalZeroDisablesPolling(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CHECK_INTERVAL", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CheckInterval != 0 {
		t.Fatalf("CheckInterval = %v, want 0", cfg.CheckInterval)
	}
}

func TestLoadCheckIntervalNegativeInvalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CHECK_INTERVAL", "-1s")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestLoadCheckIntervalInvalidFallsBack(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CHECK_INTERVAL", "not-a-duration")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CheckInterval != 5*time.Minute {
		t.Fatalf("CheckInterval = %v, want %v", cfg.CheckInterval, 5*time.Minute)
	}
}

func TestLoadHealthDefaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.HealthEnabled {
		t.Fatal("HealthEnabled = false, want true")
	}
	if cfg.HealthAddr != "127.0.0.1:8080" {
		t.Fatalf("HealthAddr = %q, want %q", cfg.HealthAddr, "127.0.0.1:8080")
	}
}

func TestLoadHealthDisabled(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HEALTH_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HealthEnabled {
		t.Fatal("HealthEnabled = true, want false")
	}
}

func TestLoadHealthEnabledRequiresAddr(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HEALTH_ADDR", " ")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestLoadHealthDisabledAllowsEmptyAddr(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HEALTH_ENABLED", "false")
	t.Setenv("HEALTH_ADDR", " ")

	_, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
}

func TestLoadDeployDelay(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DEPLOY_DELAY", "30s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DeployDelay != 30*time.Second {
		t.Fatalf("DeployDelay = %v, want %v", cfg.DeployDelay, 30*time.Second)
	}
}

func TestLoadKeepBuildsInvalidFallsBack(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("KEEP_BUILDS", "not-an-int")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.KeepBuilds != 3 {
		t.Fatalf("KeepBuilds = %d, want 3", cfg.KeepBuilds)
	}
}

func TestLoadKeepBuildsTooSmallInvalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("KEEP_BUILDS", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestLoadDeployDelayNegativeInvalid(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DEPLOY_DELAY", "-1s")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_URL", "https://example.com/repo.git")
	t.Setenv("IMAGE_NAME", "example:live")
	t.Setenv("COMPOSE_PROJECT_NAME", "example")
	t.Setenv("COMPOSE_SERVICE", "app")
}
