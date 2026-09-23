package runner

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/itsamenathan/miniploy/internal/redact"
)

func TestCommandStringRedactsURLCredentials(t *testing.T) {
	got := commandString("git", []string{"ls-remote", "https://token:secret@example.com/org/repo.git"})
	want := "git ls-remote https://REDACTED@example.com/org/repo.git"
	if got != want {
		t.Fatalf("commandString() = %q, want %q", got, want)
	}
}

func TestRunStreamingLogsProgressAndRedactsFailure(t *testing.T) {
	var logs bytes.Buffer
	r := Runner{Log: slog.New(slog.NewTextHandler(&logs, nil))}
	err := r.RunStreaming(context.Background(), "sh", "-c", "echo 'building layer'; echo 'https://user:secret@example.com/repo.git' >&2; exit 1")
	if err == nil {
		t.Fatal("RunStreaming() error = nil, want command failure")
	}
	if !strings.Contains(logs.String(), "building layer") || !strings.Contains(err.Error(), "REDACTED") {
		t.Fatalf("logs = %q, error = %v", logs.String(), err)
	}
	if strings.Contains(logs.String(), "secret") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("credentials leaked in logs or error: logs = %q, error = %v", logs.String(), err)
	}
}

func TestStreamWriterOmitsLongLines(t *testing.T) {
	var logs bytes.Buffer
	w := &streamWriter{log: slog.New(slog.NewTextHandler(&logs, nil))}
	if _, err := w.Write([]byte(strings.Repeat("a", maxOutputTail) + "https://user:secret@example.com")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("/repo.git\nnext line\n")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), "secret") || !strings.Contains(logs.String(), "next line") {
		t.Fatalf("unexpected logs after long line: %q", logs.String())
	}
}

func TestRedactURLsRedactsCredentialsInCommandOutput(t *testing.T) {
	got := redact.Text("fatal: https://token:secret@example.com/org/repo.git failed")
	want := "fatal: https://REDACTED@example.com/org/repo.git failed"
	if got != want {
		t.Fatalf("redactURLs() = %q, want %q", got, want)
	}
}
