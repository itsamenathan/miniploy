package compose

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/itsamenathan/miniploy/internal/config"
	"github.com/itsamenathan/miniploy/internal/runner"
)

type Client struct {
	cfg config.Config
	run runner.Runner
	log *slog.Logger
}

func New(cfg config.Config, log *slog.Logger) Client {
	return Client{cfg: cfg, run: runner.Runner{Log: log}, log: log}
}

func (c Client) Args(args ...string) []string {
	base := []string{"compose", "-f", c.cfg.ComposeFile, "-p", c.cfg.ComposeProjectName}
	if c.cfg.ComposeProfile != "" {
		base = append(base, "--profile", c.cfg.ComposeProfile)
	}
	return append(base, args...)
}

func (c Client) Validate(ctx context.Context) error {
	if _, err := os.Stat(c.cfg.ComposeFile); err != nil {
		return err
	}
	args := c.Args("config", "--services")
	out, err := c.run.Output(ctx, "docker", args...)
	if err != nil {
		return err
	}
	for _, service := range strings.Fields(out) {
		if service == c.cfg.ComposeService {
			return nil
		}
	}
	return fmt.Errorf("compose service %q not found in %s", c.cfg.ComposeService, c.cfg.ComposeFile)
}

// RenderedConfig returns Docker Compose's fully resolved configuration. It is
// suitable for fingerprinting all Compose inputs that affect a deployment.
func (c Client) RenderedConfig(ctx context.Context) (string, error) {
	if _, err := os.Stat(c.cfg.ComposeFile); err != nil {
		return "", err
	}
	return c.run.Output(ctx, "docker", c.Args("config")...)
}

func Hash(renderedConfig string) string {
	sum := sha256.Sum256([]byte(renderedConfig))
	return fmt.Sprintf("%x", sum)
}

func (c Client) Up(ctx context.Context) error {
	args := c.Args("up", "-d")
	args = append(args, c.cfg.RedeployArgs...)
	args = append(args, c.cfg.ComposeService)
	if c.log != nil {
		c.log.Info("redeploying compose service", "command", append([]string{"docker"}, args...))
	}
	return c.run.Run(ctx, "docker", args...)
}
