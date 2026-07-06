package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/itsamenathan/miniploy/internal/compose"
	"github.com/itsamenathan/miniploy/internal/config"
	"github.com/itsamenathan/miniploy/internal/docker"
	"github.com/itsamenathan/miniploy/internal/git"
	"github.com/itsamenathan/miniploy/internal/lock"
	"github.com/itsamenathan/miniploy/internal/state"
)

type Runner struct {
	cfg     config.Config
	log     *slog.Logger
	git     git.Client
	docker  docker.Client
	compose compose.Client
}

func New(cfg config.Config, log *slog.Logger) *Runner {
	return &Runner{
		cfg:     cfg,
		log:     log,
		git:     git.New(cfg, log),
		docker:  docker.New(cfg, log),
		compose: compose.New(cfg, log),
	}
}

func (r *Runner) Validate(ctx context.Context) error {
	if err := os.MkdirAll(r.cfg.DataDir, 0o755); err != nil {
		return err
	}
	if err := r.docker.Validate(ctx); err != nil {
		return fmt.Errorf("docker unavailable: %w", err)
	}
	if err := r.compose.Validate(ctx); err != nil {
		return fmt.Errorf("compose config invalid: %w", err)
	}
	return nil
}

func (r *Runner) RunOnce(ctx context.Context, reason string) error {
	l, err := lock.Acquire(r.cfg.LockDir)
	if err != nil {
		r.log.Warn("skipping run because lock is held", "reason", reason, "error", err)
		return nil
	}
	defer func() {
		if err := l.Release(); err != nil {
			r.log.Warn("failed to release lock", "error", err)
		}
	}()

	st, err := state.Load(r.cfg.StatePath)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	if err := r.git.EnsureRepo(ctx); err != nil {
		return fmt.Errorf("ensure repo: %w", err)
	}
	remoteCommit, err := r.git.RemoteCommit(ctx)
	if err != nil {
		return fmt.Errorf("check remote commit: %w", err)
	}

	changed := remoteCommit != st.LastDeployedCommit
	startupEnsure := reason == "startup" && r.cfg.DeployOnStart
	if !changed && !startupEnsure {
		r.log.Info("no changes detected", "commit", docker.ShortCommit(remoteCommit))
		return nil
	}

	if !changed && startupEnsure {
		r.log.Info("ensuring service is running on startup", "commit", docker.ShortCommit(remoteCommit))
		return r.compose.Up(ctx)
	}

	r.log.Info("change detected", "commit", docker.ShortCommit(remoteCommit), "previous", docker.ShortCommit(st.LastDeployedCommit))
	st.RecordAttempt(remoteCommit)
	_ = state.Save(r.cfg.StatePath, st)

	if err := r.git.Checkout(ctx); err != nil {
		st.RecordFailure(remoteCommit)
		_ = state.Save(r.cfg.StatePath, st)
		return fmt.Errorf("checkout: %w", err)
	}
	if err := r.requireDockerfile(); err != nil {
		st.RecordFailure(remoteCommit)
		_ = state.Save(r.cfg.StatePath, st)
		return err
	}

	commitImage, err := r.docker.Build(ctx, remoteCommit)
	if err != nil {
		st.RecordFailure(remoteCommit)
		_ = state.Save(r.cfg.StatePath, st)
		return fmt.Errorf("build image: %w", err)
	}
	if err := r.compose.Up(ctx); err != nil {
		st.RecordFailure(remoteCommit)
		_ = state.Save(r.cfg.StatePath, st)
		return fmt.Errorf("redeploy compose service: %w", err)
	}

	st.RecordSuccess(remoteCommit, commitImage, r.cfg.KeepBuilds)
	if err := state.Save(r.cfg.StatePath, st); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	if err := r.docker.Cleanup(ctx, st); err != nil {
		r.log.Warn("cleanup failed", "error", err)
	}
	r.log.Info("deploy successful", "commit", docker.ShortCommit(remoteCommit), "image", commitImage)
	return nil
}

func (r *Runner) requireDockerfile() error {
	path := filepath.Join(r.cfg.RepoDir, r.cfg.Dockerfile)
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("dockerfile not found at %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("dockerfile path is a directory: %s", path)
	}
	return nil
}
