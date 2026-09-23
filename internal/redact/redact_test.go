package redact

import "testing"

func TestURLRemovesCredentialsAndQuery(t *testing.T) {
	got := URL("https://user:secret@example.com/repo.git?token=private")
	want := "https://REDACTED@example.com/repo.git?REDACTED"
	if got != want {
		t.Fatalf("URL() = %q, want %q", got, want)
	}
}

func TestTextRemovesEmbeddedCredentials(t *testing.T) {
	got := Text("failed to fetch https://user:secret@example.com/repo.git")
	want := "failed to fetch https://REDACTED@example.com/repo.git"
	if got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
}
