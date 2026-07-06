package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	GitURL        string
	GitBranch     string
	GitAuthMode   string
	GitSSHKeyPath string

	ImageName    string
	Dockerfile   string
	BuildContext string

	ComposeFile        string
	ComposeProjectName string
	ComposeService     string
	ComposeProfile     string
	RedeployArgs       []string

	CheckInterval time.Duration
	KeepBuilds    int
	DeployOnStart bool
	DataDir       string
	RepoDir       string
	StatePath     string
	LockDir       string
	LogLevel      string
}

func Load() (Config, error) {
	dataDir := getenv("DATA_DIR", "/data")
	cfg := Config{
		GitURL:             os.Getenv("GIT_URL"),
		GitBranch:          getenv("GIT_BRANCH", "main"),
		GitAuthMode:        getenv("GIT_AUTH_MODE", "none"),
		GitSSHKeyPath:      os.Getenv("GIT_SSH_KEY_PATH"),
		ImageName:          os.Getenv("IMAGE_NAME"),
		Dockerfile:         getenv("DOCKERFILE", "Dockerfile"),
		BuildContext:       getenv("BUILD_CONTEXT", "."),
		ComposeFile:        getenv("COMPOSE_FILE", "/compose/docker-compose.yml"),
		ComposeProjectName: os.Getenv("COMPOSE_PROJECT_NAME"),
		ComposeService:     os.Getenv("COMPOSE_SERVICE"),
		ComposeProfile:     os.Getenv("COMPOSE_PROFILE"),
		RedeployArgs:       fields(getenv("REDEPLOY_ARGS", "--no-deps --force-recreate")),
		CheckInterval:      durationEnv("CHECK_INTERVAL", 30*time.Second),
		KeepBuilds:         intEnv("KEEP_BUILDS", 3),
		DeployOnStart:      boolEnv("DEPLOY_ON_START", true),
		DataDir:            dataDir,
		RepoDir:            getenv("REPO_DIR", dataDir+"/repo"),
		StatePath:          getenv("STATE_PATH", dataDir+"/state.json"),
		LockDir:            getenv("LOCK_DIR", dataDir+"/deploy.lock"),
		LogLevel:           getenv("LOG_LEVEL", "info"),
	}
	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	var errs []error
	required := map[string]string{
		"GIT_URL":              c.GitURL,
		"IMAGE_NAME":           c.ImageName,
		"COMPOSE_PROJECT_NAME": c.ComposeProjectName,
		"COMPOSE_SERVICE":      c.ComposeService,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			errs = append(errs, fmt.Errorf("%s is required", name))
		}
	}
	if c.GitAuthMode != "none" && c.GitAuthMode != "ssh" {
		errs = append(errs, fmt.Errorf("GIT_AUTH_MODE must be none or ssh"))
	}
	if c.GitAuthMode == "ssh" && c.GitSSHKeyPath == "" {
		errs = append(errs, fmt.Errorf("GIT_SSH_KEY_PATH is required when GIT_AUTH_MODE=ssh"))
	}
	if c.CheckInterval <= 0 {
		errs = append(errs, fmt.Errorf("CHECK_INTERVAL must be positive"))
	}
	if c.KeepBuilds < 1 {
		errs = append(errs, fmt.Errorf("KEEP_BUILDS must be at least 1"))
	}
	return errors.Join(errs...)
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func boolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func fields(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.Fields(value)
}
