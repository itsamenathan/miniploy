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

func TestLoadCheckIntervalInvalidFails(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CHECK_INTERVAL", "not-a-duration")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want invalid CHECK_INTERVAL error")
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

func TestLoadNotifyDefaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.NotifyURLs) != 0 {
		t.Fatalf("NotifyURLs = %v, want empty", cfg.NotifyURLs)
	}
	if cfg.NotifyOnSuccess {
		t.Fatal("NotifyOnSuccess = true, want false")
	}
	if !cfg.NotifyOnFailure {
		t.Fatal("NotifyOnFailure = false, want true")
	}
	if cfg.NotifyTitle != "miniploy" {
		t.Fatalf("NotifyTitle = %q, want %q", cfg.NotifyTitle, "miniploy")
	}
}

func TestLoadNotifyURLs(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("NOTIFY_URLS", "ntfy://topic, discord://token@id\nmailto://user:pass@example.com")
	t.Setenv("NOTIFY_ON", "success, failure")
	t.Setenv("NOTIFY_TITLE", "deploy bot")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	wantURLs := []string{"ntfy://topic", "discord://token@id", "mailto://user:pass@example.com"}
	if len(cfg.NotifyURLs) != len(wantURLs) {
		t.Fatalf("NotifyURLs = %v, want %v", cfg.NotifyURLs, wantURLs)
	}
	for i := range wantURLs {
		if cfg.NotifyURLs[i] != wantURLs[i] {
			t.Fatalf("NotifyURLs = %v, want %v", cfg.NotifyURLs, wantURLs)
		}
	}
	if !cfg.NotifyOnSuccess {
		t.Fatal("NotifyOnSuccess = false, want true")
	}
	if !cfg.NotifyOnFailure {
		t.Fatal("NotifyOnFailure = false, want true")
	}
	if cfg.NotifyTitle != "deploy bot" {
		t.Fatalf("NotifyTitle = %q, want %q", cfg.NotifyTitle, "deploy bot")
	}
}

func TestLoadNotifyOnAll(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("NOTIFY_URLS", "ntfy://topic")
	t.Setenv("NOTIFY_ON", "all")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.NotifyOnSuccess {
		t.Fatal("NotifyOnSuccess = false, want true")
	}
	if !cfg.NotifyOnFailure {
		t.Fatal("NotifyOnFailure = false, want true")
	}
}

func TestLoadNotifyOnInvalidWhenURLsSet(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("NOTIFY_URLS", "ntfy://topic")
	t.Setenv("NOTIFY_ON", "never")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error")
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

func TestLoadKeepBuildsInvalidFails(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("KEEP_BUILDS", "not-an-int")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want invalid KEEP_BUILDS error")
	}
}

func TestLoadInvalidBooleanFails(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DEPLOY_ON_START", "sometimes")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want invalid DEPLOY_ON_START error")
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
