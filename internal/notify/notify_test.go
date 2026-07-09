package notify

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateDisabled(t *testing.T) {
	notifier := &Notifier{}
	if err := notifier.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidateInvalidURL(t *testing.T) {
	notifier := &Notifier{urls: []string{"not-a-url"}}
	if err := notifier.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestEventTitle(t *testing.T) {
	tests := map[string]string{
		"deploy successful":   "✅ Deploy Success",
		"redeploy successful": "✅ Redeploy Success",
		"deploy failed":       "❌ Deploy Failed",
	}
	for kind, want := range tests {
		t.Run(kind, func(t *testing.T) {
			if got := (Event{Kind: kind}).Title(); got != want {
				t.Fatalf("Title() = %q, want %q", got, want)
			}
		})
	}
}

func TestEventBody(t *testing.T) {
	event := Event{
		Kind:    "deploy failed",
		Reason:  "poll",
		Commit:  "abc123",
		Image:   "example:abc123",
		Service: "app",
		Project: "example",
		Error:   errors.New("build failed"),
		Time:    time.Date(2026, 7, 9, 18, 29, 1, 0, time.UTC),
	}

	body := event.Body()
	for _, want := range []string{
		"🛠️ Project: example",
		"⚙️ Service: app",
		"❔ Reason: poll",
		"🔖 Commit: abc123",
		"📦 Image: example:abc123",
		"🕒 Date: 7/9/2026, 6:29:01 PM",
		"🚨 Error:\nbuild failed",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("Body() = %q, want to contain %q", body, want)
		}
	}
}

func TestEventBodyTrimsEmptyLines(t *testing.T) {
	body := Event{Time: time.Date(2026, 7, 9, 18, 29, 1, 0, time.UTC)}.Body()
	if body != "🕒 Date: 7/9/2026, 6:29:01 PM" {
		t.Fatalf("Body() = %q, want %q", body, "🕒 Date: 7/9/2026, 6:29:01 PM")
	}
}
