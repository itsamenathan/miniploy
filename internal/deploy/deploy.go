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
	"github.com/itsamenathan/miniploy/internal/pause"
	"github.com/itsamenathan/miniploy/internal/state"
)

type Runner struct {
	cfg         config.Config
	log         *slog.Logger
	git         git.Client
	docker      docker.Client
	compose     compose.Client
	notifier    *notify.Notifier
	stateLoader func() (state.State, error)
	stateSaver  func(state.State) error
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
	defer r.release(l)
	paused, err := pause.IsPaused(r.cfg.DataDir)
	if err != nil {
		return fmt.Errorf("check pause state: %w", err)
	}
	if paused {
		r.log.Info("automatic deployment check skipped while paused", "reason", reason)
		return nil
	}
	return r.recordCheck(ctx, r.run(ctx, reason, false))
}

func (r *Runner) Rebuild(ctx context.Context) error {
	l, err := lock.Acquire(r.cfg.LockDir)
	if err != nil {
		return err
	}
	defer r.release(l)
	return r.recordCheck(ctx, r.run(ctx, "rebuild", true))
}

// Redeploy recreates the managed service from the current image without
// rebuilding. It holds the deploy lock and records the result in state.
func (r *Runner) Redeploy(ctx context.Context) error {
	l, err := lock.Acquire(r.cfg.LockDir)
	if err != nil {
		return err
	}
	defer r.release(l)
	return r.recordCheck(ctx, r.redeploy(ctx))
}

func (r *Runner) release(l *lock.Lock) {
	if err := l.Release(); err != nil && r.log != nil {
		r.log.Warn("failed to release lock", "error", err)
	}
}

// paused reports whether automatic deployments are paused. The pause marker
// can be set at any time, including while a check is running, so automatic
// checks call this again before each step that changes the running service.
func (r *Runner) paused() bool {
	paused, err := pause.IsPaused(r.cfg.DataDir)
	if err != nil {
		r.log.Warn("could not read pause state; treating automatic checks as paused", "error", err)
		return true
	}
	return paused
}

// recordCheck stores the outcome of a check. A check that stopped because
// miniploy is shutting down is recorded as interrupted, not failed.
func (r *Runner) recordCheck(ctx context.Context, runErr error) error {
	st, loadErr := r.loadState()
	if loadErr != nil {
		return errors.Join(runErr, fmt.Errorf("load state to record check: %w", loadErr))
	}
	if interrupted(ctx, runErr) {
		st.RecordCheckInterrupted()
	} else {
		st.RecordCheck(runErr)
	}
	if saveErr := r.saveState(st); saveErr != nil {
		return errors.Join(runErr, fmt.Errorf("record check result: %w", saveErr))
	}
	return runErr
}

// interrupted reports whether err happened because ctx was cancelled.
// Commands killed by a cancelled context report their exit status rather
// than context.Canceled, so the context itself is checked.
func interrupted(ctx context.Context, err error) bool {
	return err != nil && ctx.Err() != nil
}

func (r *Runner) run(ctx context.Context, reason string, forceBuild bool) (err error) {
	automatic := !forceBuild
	notifyCommit := ""
	defer func() {
		if err != nil && !interrupted(ctx, err) {
			r.notifyFailure(reason, notifyCommit, err)
		}
	}()

	st, err := r.loadState()
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
	if automatic && (commitChanged || composeChanged) && st.IsFailedDeploy(remoteCommit, composeHash) {
		// The image for this target was already built and `compose up` failed.
		// Compose does not roll back, so rebuilding and recreating it on every
		// poll would only repeat the failure. Treat the target as current and
		// only recreate the service if it stops.
		r.log.Warn("skipping target whose last deploy failed; push a new commit, change the Compose configuration, or run miniployctl rebuild to retry",
			"commit", docker.ShortCommit(remoteCommit), "compose_hash", shortHash(composeHash))
		commitChanged = false
		composeChanged = false
	}
	startupEnsure := reason == "startup" && r.cfg.DeployOnStart
	serviceStopped := false
	if !commitChanged && !composeChanged && !startupEnsure && !forceBuild {
		running, err := r.compose.Running(ctx)
		if err != nil {
			return r.recordFailure(ctx, st, remoteCommit, fmt.Errorf("check compose service status: %w", err))
		}
		serviceStopped = !running
		if !serviceStopped {
			r.log.Info("no changes detected", "commit", docker.ShortCommit(remoteCommit), "compose_hash", shortHash(composeHash))
			return nil
		}
	}

	if !commitChanged && !forceBuild && (composeChanged || startupEnsure || serviceStopped) {
		imageExists, err := r.docker.ImageExists(ctx)
		if err != nil {
			return r.recordFailure(ctx, st, remoteCommit, fmt.Errorf("check deployed image: %w", err))
		}
		if !imageExists {
			forceBuild = true
			r.log.Warn("managed image is missing; rebuilding", "image", r.cfg.ImageName)
		}
	}

	if !commitChanged && !forceBuild {
		if composeChanged {
			r.log.Info("compose file change detected", "commit", docker.ShortCommit(remoteCommit), "compose_hash", shortHash(composeHash), "previous_compose_hash", shortHash(st.LastComposeHash))
		} else if serviceStopped {
			r.log.Warn("managed service is not running; redeploying", "service", r.cfg.ComposeService, "project", r.cfg.ComposeProjectName)
		} else {
			r.log.Info("ensuring service is running on startup", "commit", docker.ShortCommit(remoteCommit), "compose_hash", shortHash(composeHash))
		}
		st.RecordRedeployAttempt(remoteCommit)
		if err := r.saveState(st); err != nil {
			return fmt.Errorf("record redeploy attempt: %w", err)
		}
		if err := r.compose.Validate(ctx); err != nil {
			return r.recordFailure(ctx, st, remoteCommit, fmt.Errorf("compose config invalid: %w", err))
		}
		if err := r.compose.Up(ctx); err != nil {
			st.RecordFailedDeploy(remoteCommit, st.LastSuccessfulImage, composeHash)
			return r.recordFailure(ctx, st, remoteCommit, fmt.Errorf("redeploy compose service: %w", err))
		}
		r.recordRedeploySuccess(&st, remoteCommit, composeHash)
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
			return r.recordFailure(ctx, st, remoteCommit, err)
		}
		remoteCommit = debouncedCommit
		notifyCommit = remoteCommit
		composeHash = debouncedComposeHash
		if remoteCommit == st.LastDeployedCommit && composeHash == st.LastComposeHash {
			r.log.Info("no changes detected after deploy delay", "commit", docker.ShortCommit(remoteCommit), "compose_hash", shortHash(composeHash))
			return nil
		}
		if automatic && st.IsFailedDeploy(remoteCommit, composeHash) {
			r.log.Warn("skipping target whose last deploy failed", "commit", docker.ShortCommit(remoteCommit), "compose_hash", shortHash(composeHash))
			return nil
		}
	}
	if automatic && r.paused() {
		r.log.Info("automatic deployments were paused; not deploying", "commit", docker.ShortCommit(remoteCommit))
		return nil
	}
	previousStatus := st.LastStatus
	st.RecordAttempt(remoteCommit)
	if err := r.saveState(st); err != nil {
		return fmt.Errorf("record deploy attempt: %w", err)
	}

	if err := r.git.Checkout(ctx); err != nil {
		return r.recordFailure(ctx, st, remoteCommit, fmt.Errorf("checkout: %w", err))
	}
	composeHash, err = r.composeConfigHash(ctx)
	if err != nil {
		return r.recordFailure(ctx, st, remoteCommit, fmt.Errorf("hash compose config: %w", err))
	}
	if err := r.requireDockerfile(); err != nil {
		return r.recordFailure(ctx, st, remoteCommit, err)
	}
	// Validate before building so a Compose/IMAGE_NAME mismatch fails fast
	// instead of after a full image build.
	if err := r.compose.Validate(ctx); err != nil {
		return r.recordFailure(ctx, st, remoteCommit, fmt.Errorf("compose config invalid: %w", err))
	}

	commitImage, err := r.docker.Build(ctx, remoteCommit)
	if err != nil {
		return r.recordFailure(ctx, st, remoteCommit, fmt.Errorf("build image: %w", err))
	}
	if automatic && r.paused() {
		st.RestoreStatus(previousStatus)
		if err := r.saveState(st); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
		r.log.Info("automatic deployments were paused after the build; not recreating the service", "commit", docker.ShortCommit(remoteCommit), "image", commitImage)
		return nil
	}
	if err := r.compose.Up(ctx); err != nil {
		st.RecordFailedDeploy(remoteCommit, commitImage, composeHash)
		return r.recordFailure(ctx, st, remoteCommit, fmt.Errorf("redeploy compose service: %w", err))
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

// redeploy recreates the service from the current IMAGE_NAME tag.
func (r *Runner) redeploy(ctx context.Context) (err error) {
	const reason = "redeploy"
	st, err := r.loadState()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	// After a build whose `compose up` failed, IMAGE_NAME points at that build.
	commit := st.LastDeployedCommit
	if st.FailedDeployCommit != "" {
		commit = st.FailedDeployCommit
	}
	defer func() {
		if err != nil && !interrupted(ctx, err) {
			r.notifyFailure(reason, commit, err)
		}
	}()

	composeHash, err := r.composeConfigHash(ctx)
	if err != nil {
		return fmt.Errorf("hash compose file: %w", err)
	}
	st.RecordRedeployAttempt(commit)
	if err := r.saveState(st); err != nil {
		return fmt.Errorf("record redeploy attempt: %w", err)
	}
	if err := r.compose.Validate(ctx); err != nil {
		return r.recordFailure(ctx, st, commit, fmt.Errorf("compose config invalid: %w", err))
	}
	if err := r.compose.Up(ctx); err != nil {
		return r.recordFailure(ctx, st, commit, fmt.Errorf("redeploy compose service: %w", err))
	}
	r.recordRedeploySuccess(&st, commit, composeHash)
	if err := r.saveState(st); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	r.notifySuccess(reason, commit, "", "redeploy successful")
	r.log.Info("redeploy successful", "commit", docker.ShortCommit(commit), "compose_hash", shortHash(composeHash))
	return nil
}

// recordRedeploySuccess records a successful `compose up` without a build. If
// it brought up an image whose earlier deploy failed, that build becomes the
// deployed build so image cleanup keeps it.
func (r *Runner) recordRedeploySuccess(st *state.State, commit, composeHash string) {
	if st.FailedDeployCommit == commit && st.FailedDeployImage != "" && commit != st.LastDeployedCommit {
		st.RecordSuccess(commit, st.FailedDeployImage, composeHash, r.cfg.KeepBuilds)
		return
	}
	st.RecordRedeploySuccess(commit, composeHash)
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

func (r *Runner) loadState() (state.State, error) {
	if r.stateLoader != nil {
		return r.stateLoader()
	}
	return state.Load(r.cfg.StatePath)
}

func (r *Runner) saveState(st state.State) error {
	if r.stateSaver != nil {
		return r.stateSaver(st)
	}
	return state.Save(r.cfg.StatePath, st)
}

func (r *Runner) recordFailure(ctx context.Context, st state.State, commit string, failure error) error {
	if interrupted(ctx, failure) {
		st.RecordInterrupted(commit)
	} else {
		st.RecordFailure(commit, failure)
	}
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
