package git

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/itsamenathan/miniploy/internal/config"
	"github.com/itsamenathan/miniploy/internal/runner"
)

type Client struct {
	cfg config.Config
	log *slog.Logger
}

func New(cfg config.Config, log *slog.Logger) Client {
	return Client{cfg: cfg, log: log}
}

func (c Client) EnsureRepo(ctx context.Context) error {
	r, err := c.runner()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(c.cfg.RepoDir, ".git")); err == nil {
		r.Dir = c.cfg.RepoDir
		if err := r.Run(ctx, "git", "remote", "set-url", "origin", c.cfg.GitURL); err != nil {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.cfg.RepoDir), 0o755); err != nil {
		return err
	}
	return r.Run(ctx, "git", "clone", "--branch", c.cfg.GitBranch, "--single-branch", c.cfg.GitURL, c.cfg.RepoDir)
}

func (c Client) RemoteCommit(ctx context.Context) (string, error) {
	r, err := c.runner()
	if err != nil {
		return "", err
	}
	out, err := r.Output(ctx, "git", "ls-remote", c.cfg.GitURL, "refs/heads/"+c.cfg.GitBranch)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return "", fmt.Errorf("branch %q not found in remote", c.cfg.GitBranch)
	}
	return fields[0], nil
}

func (c Client) Checkout(ctx context.Context) error {
	r, err := c.runner()
	if err != nil {
		return err
	}
	r.Dir = c.cfg.RepoDir
	if err := r.Run(ctx, "git", "fetch", "origin", c.cfg.GitBranch); err != nil {
		return err
	}
	return r.Run(ctx, "git", "reset", "--hard", "origin/"+c.cfg.GitBranch)
}

func (c Client) runner() (runner.Runner, error) {
	cfg := c.cfg
	if cfg.GitAuthMode == "ssh" {
		keyPath, err := c.preparedSSHKey()
		if err != nil {
			return runner.Runner{}, err
		}
		cfg.GitSSHKeyPath = keyPath
	}
	return runner.Runner{Log: c.log, Env: gitEnv(cfg)}, nil
}

func (c Client) preparedSSHKey() (string, error) {
	info, err := os.Stat(c.cfg.GitSSHKeyPath)
	if err != nil {
		return "", err
	}
	if info.Mode().Perm()&0o077 == 0 {
		return c.cfg.GitSSHKeyPath, nil
	}

	contents, err := os.ReadFile(c.cfg.GitSSHKeyPath)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(c.cfg.DataDir, ".ssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "git_key")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func gitEnv(cfg config.Config) []string {
	if cfg.GitAuthMode != "ssh" {
		return nil
	}
	ssh := fmt.Sprintf("ssh -i %s -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new", cfg.GitSSHKeyPath)
	return []string{"GIT_SSH_COMMAND=" + ssh}
}
