package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/itsamenathan/miniploy/internal/config"
	"github.com/itsamenathan/miniploy/internal/deploy"
	"github.com/itsamenathan/miniploy/internal/health"
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

	var healthServer *health.Server
	if cfg.HealthEnabled {
		healthServer = health.New(cfg, logger)
		if err := healthServer.Start(); err != nil {
			logger.Error("health server failed to start", "error", err)
			os.Exit(2)
		}
		defer shutdownHealthServer(healthServer, logger)
	}

	if err := runner.RunOnce(ctx, "startup"); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("startup deploy check failed", "error", err)
	}

	logger.Info("miniploy started", "check_interval", cfg.CheckInterval.String(), "deploy_delay", cfg.DeployDelay.String())
	logger.Info("managed compose service", "project", cfg.ComposeProjectName, "service", cfg.ComposeService, "compose_file", cfg.ComposeFile)
	logger.Info("useful commands",
		"status", "docker compose exec miniploy miniployctl status",
		"health", "docker compose exec miniploy miniployctl health",
		"logs", "docker compose exec miniploy miniployctl logs -f",
		"redeploy", "docker compose exec miniploy miniployctl redeploy")

	if cfg.CheckInterval == 0 {
		logger.Info("polling disabled", "check_interval", cfg.CheckInterval.String())
		<-ctx.Done()
		logger.Info("shutdown requested")
		return
	}

	for {
		wait := jitteredInterval(cfg.CheckInterval)
		nextCheck := time.Now().Add(wait)
		logger.Debug("next Git/Compose check scheduled", "next_check", nextCheck.Format(time.RFC3339), "wait", wait.String())

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			logger.Info("shutdown requested")
			return
		case <-timer.C:
			if err := runner.RunOnce(ctx, "poll"); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("deploy check failed", "error", err)
			}
		}
	}
}

func jitteredInterval(interval time.Duration) time.Duration {
	jitter := interval / 10
	if jitter <= 0 {
		return interval
	}
	return interval - jitter + time.Duration(rand.Int63n(int64(2*jitter)+1))
}

func shutdownHealthServer(server *health.Server, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Warn("health server shutdown failed", "error", err)
	}
}
