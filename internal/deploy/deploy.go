package deploy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/itsamenathan/miniploy/internal/compose"
	"github.com/itsamenathan/miniploy/internal/config"
	"github.com/itsamenathan/miniploy/internal/docker"
	"github.com/itsamenathan/miniploy/internal/git"
	"github.com/itsamenathan/miniploy/internal/lock"
	"github.com/itsamenathan/miniploy/internal/notify"
	"github.com/itsamenathan/miniploy/internal/state"
)

type Runner struct {
	cfg        config.Config
	log        *slog.Logger
	git        git.Client
	docker     docker.Client
	compose    compose.Client
	notifier   *notify.Notifier
	stateSaver func(state.State) error
}

func New(cfg config.Config, log *slog.Logger) *Runner {
	return &Runner{
		cfg:      cfg,
		log:      log,
		git:      git.New(cfg, log),
		docker:   docker.New(cfg, log),
		compose:  compose.New(cfg, log),
		notifier: notify.New(cfg),
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
	if r.notifier != nil {
		if err := r.notifier.Validate(); err != nil {
			return fmt.Errorf("notification config invalid: %w", err)
		}
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
	return r.run(ctx, reason, false)
}

func (r *Runner) Rebuild(ctx context.Context) error {
	l, err := lock.Acquire(r.cfg.LockDir)
	if err != nil {
		return err
	}
	defer func() {
		if err := l.Release(); err != nil && r.log != nil {
			r.log.Warn("failed to release lock", "error", err)
		}
	}()
	return r.run(ctx, "rebuild", true)
}

func (r *Runner) run(ctx context.Context, reason string, forceBuild bool) (err error) {
	notifyCommit := ""
	defer func() {
		if err != nil {
			r.notifyFailure(reason, notifyCommit, err)
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
	notifyCommit = remoteCommit

	composeHash, err := r.composeConfigHash(ctx)
	if err != nil {
		return fmt.Errorf("hash compose file: %w", err)
	}

	commitChanged := remoteCommit != st.LastDeployedCommit
	composeChanged := composeHash != st.LastComposeHash
	startupEnsure := reason == "startup" && r.cfg.DeployOnStart
	if !commitChanged && !composeChanged && !startupEnsure && !forceBuild {
		r.log.Info("no changes detected", "commit", docker.ShortCommit(remoteCommit), "compose_hash", shortHash(composeHash))
		return nil
	}

	if !commitChanged && !forceBuild {
		if composeChanged {
			r.log.Info("compose file change detected", "commit", docker.ShortCommit(remoteCommit), "compose_hash", shortHash(composeHash), "previous_compose_hash", shortHash(st.LastComposeHash))
		} else {
			r.log.Info("ensuring service is running on startup", "commit", docker.ShortCommit(remoteCommit), "compose_hash", shortHash(composeHash))
		}
		st.RecordRedeployAttempt(remoteCommit)
		if err := r.saveState(st); err != nil {
			return fmt.Errorf("record redeploy attempt: %w", err)
		}
		if err := r.compose.Validate(ctx); err != nil {
			return r.recordFailure(st, remoteCommit, fmt.Errorf("compose config invalid: %w", err))
		}
		if err := r.compose.Up(ctx); err != nil {
			return r.recordFailure(st, remoteCommit, fmt.Errorf("redeploy compose service: %w", err))
		}
		st.RecordRedeploySuccess(remoteCommit, composeHash)
		if err := r.saveState(st); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
		r.notifySuccess(reason, remoteCommit, "", "redeploy successful")
		r.log.Info("redeploy successful", "commit", docker.ShortCommit(remoteCommit), "compose_hash", shortHash(composeHash))
		return nil
	}

	if forceBuild {
		r.log.Info("rebuild requested", "commit", docker.ShortCommit(remoteCommit), "compose_changed", composeChanged)
	} else {
		r.log.Info("change detected", "commit", docker.ShortCommit(remoteCommit), "previous", docker.ShortCommit(st.LastDeployedCommit), "compose_changed", composeChanged)
		debouncedCommit, debouncedComposeHash, err := r.debounce(ctx, remoteCommit, composeHash)
		if err != nil {
			return r.recordFailure(st, remoteCommit, err)
		}
		remoteCommit = debouncedCommit
		notifyCommit = remoteCommit
		composeHash = debouncedComposeHash
		if remoteCommit == st.LastDeployedCommit && composeHash == st.LastComposeHash {
			r.log.Info("no changes detected after deploy delay", "commit", docker.ShortCommit(remoteCommit), "compose_hash", shortHash(composeHash))
			return nil
		}
	}
	st.RecordAttempt(remoteCommit)
	if err := r.saveState(st); err != nil {
		return fmt.Errorf("record deploy attempt: %w", err)
	}

	if err := r.git.Checkout(ctx); err != nil {
		return r.recordFailure(st, remoteCommit, fmt.Errorf("checkout: %w", err))
	}
	composeHash, err = r.composeConfigHash(ctx)
	if err != nil {
		return r.recordFailure(st, remoteCommit, fmt.Errorf("hash compose config: %w", err))
	}
	if err := r.requireDockerfile(); err != nil {
		return r.recordFailure(st, remoteCommit, err)
	}

	commitImage, err := r.docker.Build(ctx, remoteCommit)
	if err != nil {
		return r.recordFailure(st, remoteCommit, fmt.Errorf("build image: %w", err))
	}
	if err := r.compose.Validate(ctx); err != nil {
		return r.recordFailure(st, remoteCommit, fmt.Errorf("compose config invalid: %w", err))
	}
	if err := r.compose.Up(ctx); err != nil {
		return r.recordFailure(st, remoteCommit, fmt.Errorf("redeploy compose service: %w", err))
	}

	st.RecordSuccess(remoteCommit, commitImage, composeHash, r.cfg.KeepBuilds)
	if err := r.saveState(st); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	if err := r.docker.Cleanup(ctx, st); err != nil {
		r.log.Warn("cleanup failed", "error", err)
	}
	r.notifySuccess(reason, remoteCommit, commitImage, "deploy successful")
	r.log.Info("deploy successful", "commit", docker.ShortCommit(remoteCommit), "image", commitImage)
	return nil
}

func (r *Runner) notifySuccess(reason, commit, image, kind string) {
	if r.notifier == nil || !r.notifier.Enabled() {
		return
	}
	event := notify.Event{
		Kind:    kind,
		Reason:  reason,
		Commit:  docker.ShortCommit(commit),
		Image:   image,
		Service: r.cfg.ComposeService,
		Project: r.cfg.ComposeProjectName,
	}
	if err := r.notifier.Success(event); err != nil {
		r.log.Warn("deployment notification failed", "event", kind, "error", err)
	}
}

func (r *Runner) notifyFailure(reason, commit string, failure error) {
	if r.notifier == nil || !r.notifier.Enabled() {
		return
	}
	event := notify.Event{
		Kind:    "deploy failed",
		Reason:  reason,
		Commit:  docker.ShortCommit(commit),
		Service: r.cfg.ComposeService,
		Project: r.cfg.ComposeProjectName,
		Error:   failure,
	}
	if err := r.notifier.Failure(event); err != nil {
		r.log.Warn("deployment notification failed", "event", "deploy failed", "error", err)
	}
}

func (r *Runner) debounce(ctx context.Context, commit, composeHash string) (string, string, error) {
	if r.cfg.DeployDelay == 0 {
		return commit, composeHash, nil
	}

	r.log.Info("waiting before deploy", "delay", r.cfg.DeployDelay.String(), "commit", docker.ShortCommit(commit))
	timer := time.NewTimer(r.cfg.DeployDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return "", "", ctx.Err()
	case <-timer.C:
	}

	latestCommit, err := r.git.RemoteCommit(ctx)
	if err != nil {
		return "", "", fmt.Errorf("recheck remote commit after deploy delay: %w", err)
	}
	latestComposeHash, err := r.composeConfigHash(ctx)
	if err != nil {
		return "", "", fmt.Errorf("hash compose file after deploy delay: %w", err)
	}
	if latestCommit != commit || latestComposeHash != composeHash {
		r.log.Info("deploy target updated during delay", "commit", docker.ShortCommit(latestCommit), "previous", docker.ShortCommit(commit), "compose_changed", latestComposeHash != composeHash)
	}
	return latestCommit, latestComposeHash, nil
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

func (r *Runner) composeConfigHash(ctx context.Context) (string, error) {
	contents, err := r.compose.RenderedConfig(ctx)
	if err != nil {
		return "", err
	}
	return compose.Hash(contents), nil
}

func (r *Runner) saveState(st state.State) error {
	if r.stateSaver != nil {
		return r.stateSaver(st)
	}
	return state.Save(r.cfg.StatePath, st)
}

func (r *Runner) recordFailure(st state.State, commit string, failure error) error {
	st.RecordFailure(commit, failure)
	if err := r.saveState(st); err != nil {
		return errors.Join(failure, fmt.Errorf("record deploy failure: %w", err))
	}
	return failure
}

func shortHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}
