package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/itsamenathan/miniploy/internal/config"
	"github.com/itsamenathan/miniploy/internal/deploy"
	"github.com/itsamenathan/miniploy/internal/logging"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(2)
	}

	logger := logging.New(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner := deploy.New(cfg, logger)
	if err := runner.Validate(ctx); err != nil {
		logger.Error("validation failed", "error", err)
		os.Exit(2)
	}

	if err := runner.RunOnce(ctx, "startup"); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("startup deploy check failed", "error", err)
	}

	ticker := time.NewTicker(cfg.CheckInterval)
	defer ticker.Stop()

	logger.Info("miniploy started", "check_interval", cfg.CheckInterval.String())
	logger.Info("managed compose service", "project", cfg.ComposeProjectName, "service", cfg.ComposeService, "compose_file", cfg.ComposeFile)
	logger.Info("useful commands",
		"status", "docker compose exec miniploy miniployctl status",
		"logs", "docker compose exec miniploy miniployctl logs -f",
		"redeploy", "docker compose exec miniploy miniployctl redeploy")
	for {
		select {
		case <-ctx.Done():
			logger.Info("shutdown requested")
			return
		case <-ticker.C:
			if err := runner.RunOnce(ctx, "poll"); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("deploy check failed", "error", err)
			}
		}
	}
}
