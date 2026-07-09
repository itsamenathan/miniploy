package notify

import (
	"errors"
	"fmt"
	"strings"
	"time"

	apprise "github.com/unraid/apprise-go"

	"github.com/itsamenathan/miniploy/internal/config"
)

// Event describes a deployment notification event.
type Event struct {
	Kind    string
	Reason  string
	Commit  string
	Image   string
	Service string
	Project string
	Error   error
	Time    time.Time
}

// Notifier sends deployment notifications to configured targets.
type Notifier struct {
	urls      []string
	title     string
	onSuccess bool
	onFailure bool
}

// New creates a notifier from configuration.
func New(cfg config.Config) *Notifier {
	return &Notifier{
		urls:      append([]string(nil), cfg.NotifyURLs...),
		title:     cfg.NotifyTitle,
		onSuccess: cfg.NotifyOnSuccess,
		onFailure: cfg.NotifyOnFailure,
	}
}

// Enabled reports whether notification targets are configured.
func (n *Notifier) Enabled() bool {
	return n != nil && len(n.urls) > 0
}

// Validate checks configured notification URLs without sending a notification.
func (n *Notifier) Validate() error {
	if !n.Enabled() {
		return nil
	}
	client := apprise.New()
	if err := client.AddAll(n.urls...); err != nil {
		return errors.New("invalid NOTIFY_URLS entry")
	}
	return nil
}

// Success sends a success notification when enabled by configuration.
func (n *Notifier) Success(event Event) error {
	if !n.Enabled() || !n.onSuccess {
		return nil
	}
	return n.send(event, apprise.NotifySuccess)
}

// Failure sends a failure notification when enabled by configuration.
func (n *Notifier) Failure(event Event) error {
	if !n.Enabled() || !n.onFailure {
		return nil
	}
	return n.send(event, apprise.NotifyFailure)
}

func (n *Notifier) send(event Event, notifyType apprise.NotifyType) error {
	body := event.Body()
	if source := strings.TrimSpace(n.title); source != "" {
		body += "\n🤖 Source: " + source
	}
	title := event.Title()
	if err := apprise.Send(n.urls, body, apprise.WithTitle(title), apprise.WithNotifyType(notifyType)); err != nil {
		return errors.New("notification delivery failed for one or more targets")
	}
	return nil
}

// Title formats the notification title.
func (e Event) Title() string {
	switch strings.ToLower(strings.TrimSpace(e.Kind)) {
	case "deploy successful":
		return "✅ Deploy Success"
	case "redeploy successful":
		return "✅ Redeploy Success"
	case "deploy failed":
		return "❌ Deploy Failed"
	default:
		if strings.TrimSpace(e.Kind) == "" {
			return "🚀 Miniploy"
		}
		return "🚀 " + strings.TrimSpace(e.Kind)
	}
}

// Body formats the notification body.
func (e Event) Body() string {
	var b strings.Builder
	if strings.TrimSpace(e.Project) != "" {
		fmt.Fprintf(&b, "🛠️ Project: %s\n", e.Project)
	}
	if strings.TrimSpace(e.Service) != "" {
		fmt.Fprintf(&b, "⚙️ Service: %s\n", e.Service)
	}
	if strings.TrimSpace(e.Reason) != "" {
		fmt.Fprintf(&b, "❔ Reason: %s\n", e.Reason)
	}
	if strings.TrimSpace(e.Commit) != "" {
		fmt.Fprintf(&b, "🔖 Commit: %s\n", e.Commit)
	}
	if strings.TrimSpace(e.Image) != "" {
		fmt.Fprintf(&b, "📦 Image: %s\n", e.Image)
	}
	fmt.Fprintf(&b, "🕒 Date: %s\n", eventTime(e).Format("1/2/2006, 3:04:05 PM"))
	if e.Error != nil {
		fmt.Fprintf(&b, "\n🚨 Error:\n%s\n", e.Error.Error())
	}
	return strings.TrimSpace(b.String())
}

func eventTime(e Event) time.Time {
	if e.Time.IsZero() {
		return time.Now()
	}
	return e.Time
}
