package runner

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
)

type Runner struct {
	Log *slog.Logger
	Env []string
	Dir string
}

func (r Runner) Run(ctx context.Context, name string, args ...string) error {
	_, err := r.Output(ctx, name, args...)
	return err
}

func (r Runner) Output(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if r.Dir != "" {
		cmd.Dir = r.Dir
	}
	if len(r.Env) > 0 {
		cmd.Env = append(cmd.Environ(), r.Env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if r.Log != nil {
		r.Log.Debug("running command", "cmd", commandString(name, args))
	}
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(stdout.String()), fmt.Errorf("%s failed: %w: %s", commandString(name, args), err, redactURLs(strings.TrimSpace(stderr.String())))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func commandString(name string, args []string) string {
	parts := append([]string{name}, args...)
	for i, part := range parts {
		parts[i] = redactURL(part)
	}
	return strings.Join(parts, " ")
}

var urlPattern = regexp.MustCompile(`https?://[^\s'\"]+`)

func redactURLs(value string) string {
	return urlPattern.ReplaceAllStringFunc(value, redactURL)
}

func redactURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.User == nil {
		return value
	}
	parsed.User = url.User("REDACTED")
	return parsed.String()
}
