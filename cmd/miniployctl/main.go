package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/itsamenathan/miniploy/internal/compose"
	"github.com/itsamenathan/miniploy/internal/config"
	"github.com/itsamenathan/miniploy/internal/deploy"
	"github.com/itsamenathan/miniploy/internal/logging"
	"github.com/itsamenathan/miniploy/internal/state"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "miniployctl: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		usage()
		return nil
	}
	if len(args) > 1 && (args[1] == "help" || args[1] == "--help" || args[1] == "-h") {
		commandUsage(args[0])
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}
	client := compose.New(cfg, nil)

	switch args[0] {
	case "status":
		return status(ctx, cfg, client)
	case "health":
		return health(ctx, cfg)
	case "logs":
		return logs(ctx, cfg, client, args[1:])
	case "ps":
		return docker(ctx, client.Args("ps", cfg.ComposeService))
	case "restart":
		return docker(ctx, client.Args("restart", cfg.ComposeService))
	case "stop":
		return docker(ctx, client.Args("stop", cfg.ComposeService))
	case "start":
		return docker(ctx, client.Args("up", "-d", cfg.ComposeService))
	case "redeploy":
		redeployArgs := client.Args("up", "-d")
		redeployArgs = append(redeployArgs, cfg.RedeployArgs...)
		redeployArgs = append(redeployArgs, cfg.ComposeService)
		return docker(ctx, redeployArgs)
	case "rebuild":
		logger := logging.New(cfg.LogLevel)
		runner := deploy.New(cfg, logger)
		return runner.Rebuild(ctx)
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func health(ctx context.Context, cfg config.Config) error {
	if !cfg.HealthEnabled {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL(cfg.HealthAddr), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health endpoint returned %s", resp.Status)
	}
	return nil
}

func healthURL(addr string) string {
	addr = strings.TrimSpace(addr)
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/") + "/healthz"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	if strings.HasPrefix(addr, "0.0.0.0:") {
		addr = strings.Replace(addr, "0.0.0.0:", "127.0.0.1:", 1)
	}
	if strings.HasPrefix(addr, "[::]:") {
		addr = strings.Replace(addr, "[::]:", "127.0.0.1:", 1)
	}
	return "http://" + addr + "/healthz"
}

func logs(ctx context.Context, cfg config.Config, client compose.Client, args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	follow := fs.Bool("f", false, "follow log output")
	tail := fs.String("tail", "", "number of lines to show")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("logs does not accept positional arguments")
	}

	logArgs := []string{"logs"}
	if *follow {
		logArgs = append(logArgs, "-f")
	}
	if *tail != "" {
		logArgs = append(logArgs, "--tail", *tail)
	}
	logArgs = append(logArgs, cfg.ComposeService)
	return docker(ctx, client.Args(logArgs...))
}

func status(ctx context.Context, cfg config.Config, client compose.Client) error {
	st, err := state.Load(cfg.StatePath)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	fmt.Printf("miniploy\n")
	fmt.Printf("  compose project: %s\n", cfg.ComposeProjectName)
	fmt.Printf("  compose service: %s\n", cfg.ComposeService)
	fmt.Printf("  compose file:    %s\n", cfg.ComposeFile)
	if cfg.ComposeProfile != "" {
		fmt.Printf("  compose profile: %s\n", cfg.ComposeProfile)
	}
	fmt.Printf("  image:           %s\n", cfg.ImageName)
	fmt.Printf("  git:             %s (%s)\n", cfg.GitURL, cfg.GitBranch)
	fmt.Printf("  check interval:  %s\n", pollingInterval(cfg))
	fmt.Printf("  deploy delay:    %s\n", cfg.DeployDelay.String())
	fmt.Printf("  state path:      %s\n", cfg.StatePath)
	fmt.Println()
	fmt.Printf("state\n")
	fmt.Printf("  status:          %s\n", valueOrDash(st.LastStatus))
	fmt.Printf("  deployed commit: %s\n", valueOrDash(st.LastDeployedCommit))
	fmt.Printf("  attempted commit: %s\n", valueOrDash(st.LastAttemptedCommit))
	fmt.Printf("  image:           %s\n", valueOrDash(st.LastSuccessfulImage))
	fmt.Printf("  compose hash:    %s\n", valueOrDash(shortValue(st.LastComposeHash, 12)))
	if !st.LastErrorAt.IsZero() {
		fmt.Printf("  last error at:   %s\n", st.LastErrorAt.Format("2006-01-02 15:04:05 MST"))
	}
	if st.LastError != "" {
		fmt.Printf("  last error:      %s\n", st.LastError)
	}
	if !st.Updated.IsZero() {
		fmt.Printf("  updated:         %s\n", st.Updated.Format("2006-01-02 15:04:05 MST"))
	}
	fmt.Println()
	fmt.Printf("compose ps\n")
	return docker(ctx, client.Args("ps", cfg.ComposeService))
}

func docker(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func pollingInterval(cfg config.Config) string {
	if cfg.CheckInterval == 0 {
		return "disabled"
	}
	return cfg.CheckInterval.String()
}

func shortValue(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: miniployctl <command> [options]

Commands:
  status             Show miniploy config, deploy state, and app container status
  health             Check the local miniploy health endpoint
  logs [-f] [--tail N]
                     Show logs for the managed app service
  ps                 Show Compose status for the managed app service
  restart            Restart the managed app service
  stop               Stop the managed app service
  start              Start the managed app service
  redeploy           Recreate the managed app service with REDEPLOY_ARGS
  rebuild            Rebuild the watched Git commit, then recreate the service
  help               Show this help text

Command help:
  miniployctl help
  miniployctl logs --help
  miniployctl rebuild --help

Run inside the miniploy container, for example:
  docker compose exec miniploy miniployctl logs -f
  docker compose exec miniploy miniployctl redeploy
  docker compose exec miniploy miniployctl rebuild
`)
}

func commandUsage(command string) {
	switch command {
	case "logs":
		fmt.Fprintf(os.Stderr, "Usage: miniployctl logs [-f] [--tail N]\n\nShow logs for the managed app service.\n")
	case "status":
		fmt.Fprintf(os.Stderr, "Usage: miniployctl status\n\nShow miniploy config, deploy state, and app container status.\n")
	case "health":
		fmt.Fprintf(os.Stderr, "Usage: miniployctl health\n\nCheck the local miniploy health endpoint.\n")
	case "ps":
		fmt.Fprintf(os.Stderr, "Usage: miniployctl ps\n\nShow Compose status for the managed app service.\n")
	case "restart", "stop", "start":
		fmt.Fprintf(os.Stderr, "Usage: miniployctl %s\n\nControl the managed app service with Docker Compose.\n", command)
	case "redeploy":
		fmt.Fprintf(os.Stderr, "Usage: miniployctl redeploy\n\nRecreate the managed app service with REDEPLOY_ARGS. Does not rebuild the image.\n")
	case "rebuild":
		fmt.Fprintf(os.Stderr, "Usage: miniployctl rebuild\n\nFetch the watched Git branch, rebuild the current remote commit, and recreate the managed app service.\n")
	default:
		usage()
	}
}
