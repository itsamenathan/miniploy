package deploy

import (
	"context"
	"crypto/sha256"
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

	composeHash, err := r.composeFileHash()
	if err != nil {
		return fmt.Errorf("hash compose file: %w", err)
	}

	commitChanged := remoteCommit != st.LastDeployedCommit
	composeChanged := composeHash != st.LastComposeHash
	startupEnsure := reason == "startup" && r.cfg.DeployOnStart
	if !commitChanged && !composeChanged && !startupEnsure {
		r.log.Info("no changes detected", "commit", docker.ShortCommit(remoteCommit), "compose_hash", shortHash(composeHash))
		return nil
	}

	if !commitChanged {
		if composeChanged {
			r.log.Info("compose file change detected", "commit", docker.ShortCommit(remoteCommit), "compose_hash", shortHash(composeHash), "previous_compose_hash", shortHash(st.LastComposeHash))
		} else {
			r.log.Info("ensuring service is running on startup", "commit", docker.ShortCommit(remoteCommit), "compose_hash", shortHash(composeHash))
		}
		st.RecordRedeployAttempt(remoteCommit)
		_ = state.Save(r.cfg.StatePath, st)
		if err := r.compose.Validate(ctx); err != nil {
			st.RecordFailure(remoteCommit)
			_ = state.Save(r.cfg.StatePath, st)
			return fmt.Errorf("compose config invalid: %w", err)
		}
		if err := r.compose.Up(ctx); err != nil {
			st.RecordFailure(remoteCommit)
			_ = state.Save(r.cfg.StatePath, st)
			return fmt.Errorf("redeploy compose service: %w", err)
		}
		st.RecordRedeploySuccess(remoteCommit, composeHash)
		if err := state.Save(r.cfg.StatePath, st); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
		r.log.Info("redeploy successful", "commit", docker.ShortCommit(remoteCommit), "compose_hash", shortHash(composeHash))
		return nil
	}

	r.log.Info("change detected", "commit", docker.ShortCommit(remoteCommit), "previous", docker.ShortCommit(st.LastDeployedCommit), "compose_changed", composeChanged)
	st.RecordAttempt(remoteCommit)
	_ = state.Save(r.cfg.StatePath, st)

	if err := r.git.Checkout(ctx); err != nil {
		st.RecordFailure(remoteCommit)
		_ = state.Save(r.cfg.StatePath, st)
		return fmt.Errorf("checkout: %w", err)
	}
	composeHash, err = r.composeFileHash()
	if err != nil {
		st.RecordFailure(remoteCommit)
		_ = state.Save(r.cfg.StatePath, st)
		return fmt.Errorf("hash compose file: %w", err)
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
	if err := r.compose.Validate(ctx); err != nil {
		st.RecordFailure(remoteCommit)
		_ = state.Save(r.cfg.StatePath, st)
		return fmt.Errorf("compose config invalid: %w", err)
	}
	if err := r.compose.Up(ctx); err != nil {
		st.RecordFailure(remoteCommit)
		_ = state.Save(r.cfg.StatePath, st)
		return fmt.Errorf("redeploy compose service: %w", err)
	}

	st.RecordSuccess(remoteCommit, commitImage, composeHash, r.cfg.KeepBuilds)
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

func (r *Runner) composeFileHash() (string, error) {
	contents, err := os.ReadFile(r.cfg.ComposeFile)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(contents)
	return fmt.Sprintf("%x", sum), nil
}

func shortHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}
