package docker

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/itsamenathan/miniploy/internal/config"
	"github.com/itsamenathan/miniploy/internal/runner"
	"github.com/itsamenathan/miniploy/internal/state"
)

type Client struct {
	cfg config.Config
	run runner.Runner
}

func New(cfg config.Config, log *slog.Logger) Client {
	return Client{cfg: cfg, run: runner.Runner{Log: log, Env: []string{"DOCKER_BUILDKIT=1", "BUILDKIT_PROGRESS=plain"}}}
}

func (c Client) Validate(ctx context.Context) error {
	return c.run.Run(ctx, "docker", "version")
}

func (c Client) Build(ctx context.Context, commit string) (string, error) {
	short := ShortCommit(commit)
	commitImage := CommitImage(c.cfg.ImageName, short)
	contextPath := filepath.Join(c.cfg.RepoDir, c.cfg.BuildContext)
	dockerfilePath := filepath.Join(c.cfg.RepoDir, c.cfg.Dockerfile)

	args := []string{
		"build",
		"-f", dockerfilePath,
		"-t", c.cfg.ImageName,
		"-t", commitImage,
		"--label", "miniploy.managed=true",
		"--label", "miniploy.project=" + c.cfg.ComposeProjectName,
		"--label", "miniploy.service=" + c.cfg.ComposeService,
		"--label", "miniploy.commit=" + commit,
		contextPath,
	}
	return commitImage, c.run.Run(ctx, "docker", args...)
}

func (c Client) Cleanup(ctx context.Context, st state.State) error {
	keep := map[string]bool{c.cfg.ImageName: true, st.LastSuccessfulImage: true}
	for _, build := range st.Builds {
		keep[build.Image] = true
	}
	out, err := c.run.Output(ctx, "docker", "image", "ls",
		"--filter", "label=miniploy.managed=true",
		"--filter", "label=miniploy.project="+c.cfg.ComposeProjectName,
		"--filter", "label=miniploy.service="+c.cfg.ComposeService,
		"--format", "{{.Repository}}:{{.Tag}}")
	if err != nil {
		return err
	}
	for _, image := range strings.Fields(out) {
		if image == "<none>:<none>" || keep[image] {
			continue
		}
		if err := c.run.Run(ctx, "docker", "image", "rm", image); err != nil {
			return fmt.Errorf("remove old image %s: %w", image, err)
		}
	}
	return nil
}

func CommitImage(imageName, shortCommit string) string {
	lastSlash := strings.LastIndex(imageName, "/")
	lastColon := strings.LastIndex(imageName, ":")
	if lastColon > lastSlash {
		return imageName[:lastColon+1] + shortCommit
	}
	return imageName + ":" + shortCommit
}

func ShortCommit(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
