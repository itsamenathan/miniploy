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

// RunStreaming writes command progress to the logger and retains a short
// tail for errors. Use it for long-running commands such as builds and
// `compose up --wait`.
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
		if tail := output.Tail(); tail != "" {
			return fmt.Errorf("%s failed: %w: %s", commandString(name, args), err, tail)
		}
		return fmt.Errorf("%s failed: %w", commandString(name, args), err)
	}
	return nil
}

// maxLineLength bounds a single buffered output line.
const maxLineLength = 32 * 1024

// maxErrorTail bounds the command output included in returned errors. Errors
// end up in notifications, state.json, and /status, so they must stay small.
const maxErrorTail = 2 * 1024

type streamWriter struct {
	mu          sync.Mutex
	log         *slog.Logger
	pending     []byte
	discardLine bool
	lines       []string
	tailBytes   int
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
			w.emit(strings.TrimRight(string(w.pending), "\r"))
		}
		w.pending = w.pending[:0]
		w.discardLine = false
		p = p[end+1:]
	}
	return length, nil
}

func (w *streamWriter) appendSegment(segment []byte) {
	if w.discardLine {
		return
	}
	if len(w.pending)+len(segment) > maxLineLength {
		w.emit("[command output line exceeded 32 KiB and was omitted]")
		w.pending = w.pending[:0]
		w.discardLine = true
		return
	}
	w.pending = append(w.pending, segment...)
}

func (w *streamWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) > 0 && !w.discardLine {
		w.emit(string(w.pending))
		w.pending = w.pending[:0]
	}
}

func (w *streamWriter) emit(line string) {
	line = redact.Text(line)
	if line == "" {
		return
	}
	if w.log != nil {
		w.log.Info("command output", "line", line)
	}
	w.lines = append(w.lines, line)
	w.tailBytes += len(line) + 1
	drop := 0
	for w.tailBytes > maxErrorTail && len(w.lines)-drop > 1 {
		w.tailBytes -= len(w.lines[drop]) + 1
		drop++
	}
	if drop > 0 {
		// Copy forward so the backing array does not grow without bound.
		n := copy(w.lines, w.lines[drop:])
		clear(w.lines[n:])
		w.lines = w.lines[:n]
	}
}

// Tail returns the most recent output, at most maxErrorTail bytes of valid
// UTF-8.
func (w *streamWriter) Tail() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	tail := strings.Join(w.lines, "\n")
	if len(tail) > maxErrorTail {
		tail = strings.ToValidUTF8(tail[len(tail)-maxErrorTail:], "")
	}
	return tail
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
