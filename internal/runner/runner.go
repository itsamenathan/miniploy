package runner

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"

	"github.com/itsamenathan/miniploy/internal/redact"
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

// RunStreaming writes command progress to the logger and retains a bounded
// tail for errors. Use it for builds, which can take a long time.
func (r Runner) RunStreaming(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if r.Dir != "" {
		cmd.Dir = r.Dir
	}
	if len(r.Env) > 0 {
		cmd.Env = append(cmd.Environ(), r.Env...)
	}
	output := &streamWriter{log: r.Log}
	cmd.Stdout = output
	cmd.Stderr = output
	if r.Log != nil {
		r.Log.Debug("running command", "cmd", commandString(name, args))
	}
	err := cmd.Run()
	output.flush()
	if err != nil {
		return fmt.Errorf("%s failed: %w: %s", commandString(name, args), err, output.tail)
	}
	return nil
}

const maxOutputTail = 32 * 1024

type streamWriter struct {
	mu          sync.Mutex
	log         *slog.Logger
	pending     string
	discardLine bool
	tail        string
}

func (w *streamWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	length := len(p)
	for len(p) > 0 {
		end := bytes.IndexByte(p, '\n')
		if end < 0 {
			w.appendSegment(p)
			break
		}
		w.appendSegment(p[:end])
		if !w.discardLine {
			w.emit(strings.TrimRight(w.pending, "\r"))
		}
		w.pending = ""
		w.discardLine = false
		p = p[end+1:]
	}
	return length, nil
}

func (w *streamWriter) appendSegment(segment []byte) {
	if w.discardLine {
		return
	}
	if len(w.pending)+len(segment) > maxOutputTail {
		w.emit("[build output line exceeded 32 KiB and was omitted]")
		w.pending = ""
		w.discardLine = true
		return
	}
	w.pending += string(segment)
}

func (w *streamWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending != "" && !w.discardLine {
		w.emit(w.pending)
		w.pending = ""
	}
}

func (w *streamWriter) emit(line string) {
	line = redact.Text(line)
	if w.log != nil && line != "" {
		w.log.Info("build output", "line", line)
	}
	w.tail += line + "\n"
	if len(w.tail) > maxOutputTail {
		w.tail = w.tail[len(w.tail)-maxOutputTail:]
	}
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
		return strings.TrimSpace(stdout.String()), fmt.Errorf("%s failed: %w: %s", commandString(name, args), err, redact.Text(strings.TrimSpace(stderr.String())))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func commandString(name string, args []string) string {
	parts := append([]string{name}, args...)
	for i, part := range parts {
		parts[i] = redact.Text(part)
	}
	return strings.Join(parts, " ")
}
