package runner

import "testing"

func TestCommandStringRedactsURLCredentials(t *testing.T) {
	got := commandString("git", []string{"ls-remote", "https://token:secret@example.com/org/repo.git"})
	want := "git ls-remote https://REDACTED@example.com/org/repo.git"
	if got != want {
		t.Fatalf("commandString() = %q, want %q", got, want)
	}
}

func TestRedactURLsRedactsCredentialsInCommandOutput(t *testing.T) {
	got := redactURLs("fatal: https://token:secret@example.com/org/repo.git failed")
	want := "fatal: https://REDACTED@example.com/org/repo.git failed"
	if got != want {
		t.Fatalf("redactURLs() = %q, want %q", got, want)
	}
}
