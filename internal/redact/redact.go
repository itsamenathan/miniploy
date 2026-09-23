package redact

import (
	"net/url"
	"regexp"
)

var urlPattern = regexp.MustCompile(`https?://[^\s'\"]+`)

// URL removes credentials and query parameters from an HTTP URL.
func URL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return value
	}
	if parsed.User != nil {
		parsed.User = url.User("REDACTED")
	}
	if parsed.RawQuery != "" {
		parsed.RawQuery = "REDACTED"
	}
	parsed.Fragment = ""
	return parsed.String()
}

// Text redacts HTTP URLs embedded in command output and saved errors.
func Text(value string) string {
	return urlPattern.ReplaceAllStringFunc(value, URL)
}
