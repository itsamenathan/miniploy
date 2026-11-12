package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Build struct {
	Commit   string    `json:"commit"`
	Image    string    `json:"image"`
	Deployed time.Time `json:"deployed"`
}

type State struct {
	LastDeployedCommit  string    `json:"lastDeployedCommit"`
	LastSuccessfulImage string    `json:"lastSuccessfulImage"`
	LastAttemptedCommit string    `json:"lastAttemptedCommit"`
	LastComposeHash     string    `json:"lastComposeHash"`
	LastStatus          string    `json:"lastStatus"`
	LastError           string    `json:"lastError,omitempty"`
	LastErrorAt         time.Time `json:"lastErrorAt,omitempty"`
	LastCheckStatus     string    `json:"lastCheckStatus,omitempty"`
	LastCheckError      string    `json:"lastCheckError,omitempty"`
	LastCheckAt         time.Time `json:"lastCheckAt,omitempty"`
	// FailedDeploy* identify a target whose image was built (or whose
	// configuration was applied) but whose `docker compose up --wait` failed.
	// Compose does not roll back, so the target may already be serving.
	// Automatic checks do not rebuild or recreate it again until the Git commit
	// or Compose configuration changes, or the operator runs a manual rebuild.
	FailedDeployCommit      string    `json:"failedDeployCommit,omitempty"`
	FailedDeployImage       string    `json:"failedDeployImage,omitempty"`
	FailedDeployComposeHash string    `json:"failedDeployComposeHash,omitempty"`
	Builds                  []Build   `json:"builds"`
	Updated                 time.Time `json:"updated"`
}

// MaxErrorLength bounds error text persisted in state so that long command
// output cannot bloat state.json or the /status response.
const MaxErrorLength = 4096

// TruncateError keeps the end of long error text, which usually holds the
// most useful command output, and ensures the result is valid UTF-8.
func TruncateError(text string, max int) string {
	if max <= 0 || len(text) <= max {
		return text
	}
	const marker = "[truncated] ..."
	if max <= len(marker) {
		return strings.ToValidUTF8(text[len(text)-max:], "")
	}
	return marker + strings.ToValidUTF8(text[len(text)-(max-len(marker)):], "")
}

func (s *State) RecordCheck(err error) {
	s.LastCheckAt = time.Now().UTC()
	if err != nil {
		s.LastCheckStatus = "failed"
		s.LastCheckError = TruncateError(err.Error(), MaxErrorLength)
		return
	}
	s.LastCheckStatus = "success"
	s.LastCheckError = ""
}

// RecordCheckInterrupted records a check that stopped because miniploy was
// shutting down. It is not reported as a failure.
func (s *State) RecordCheckInterrupted() {
	s.LastCheckAt = time.Now().UTC()
	s.LastCheckStatus = "interrupted"
	s.LastCheckError = ""
}

func (s State) EffectiveStatus() string {
	if s.LastCheckStatus == "failed" {
		return "failed"
	}
	if s.LastStatus == "" {
		return "unknown"
	}
	return s.LastStatus
}

func Load(path string) (State, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	var st State
	if err := json.Unmarshal(contents, &st); err != nil {
		return State{}, err
	}
	return st, nil
}

// Save writes state atomically. Updated is set by the Record* methods that
// change deployment state, not by Save, so it reflects the last deployment
// change rather than the last poll.
func Save(path string, st State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	contents, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, contents, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *State) touch() {
	s.Updated = time.Now().UTC()
}

func (s *State) RecordAttempt(commit string) {
	s.LastAttemptedCommit = commit
	s.LastStatus = "building"
	s.touch()
}

func (s *State) RecordFailure(commit string, err error) {
	s.LastAttemptedCommit = commit
	s.LastStatus = "failed"
	if err != nil {
		s.LastError = TruncateError(err.Error(), MaxErrorLength)
		s.LastErrorAt = time.Now().UTC()
	}
	s.touch()
}

// RecordInterrupted records a deployment that stopped because miniploy was
// shutting down. The next check retries it.
func (s *State) RecordInterrupted(commit string) {
	s.LastAttemptedCommit = commit
	s.LastStatus = "interrupted"
	s.touch()
}

// RecordFailedDeploy remembers a target whose `compose up` failed after its
// image was built, so automatic checks do not rebuild it on every poll.
func (s *State) RecordFailedDeploy(commit, image, composeHash string) {
	s.FailedDeployCommit = commit
	s.FailedDeployImage = image
	s.FailedDeployComposeHash = composeHash
}

// IsFailedDeploy reports whether commit and composeHash match a target whose
// deployment already failed after building.
func (s State) IsFailedDeploy(commit, composeHash string) bool {
	return s.FailedDeployCommit != "" && s.FailedDeployCommit == commit && s.FailedDeployComposeHash == composeHash
}

func (s *State) clearFailedDeploy() {
	s.FailedDeployCommit = ""
	s.FailedDeployImage = ""
	s.FailedDeployComposeHash = ""
}

// RestoreStatus sets LastStatus back to a previous value, for an attempt that
// stopped before changing anything.
func (s *State) RestoreStatus(status string) {
	s.LastStatus = status
	s.touch()
}

func (s *State) RecordRedeployAttempt(commit string) {
	s.LastAttemptedCommit = commit
	s.LastStatus = "redeploying"
	s.touch()
}

func (s *State) RecordRedeploySuccess(commit, composeHash string) {
	s.LastAttemptedCommit = commit
	s.LastDeployedCommit = commit
	s.LastComposeHash = composeHash
	s.LastStatus = "success"
	s.LastError = ""
	s.LastErrorAt = time.Time{}
	s.clearFailedDeploy()
	s.touch()
}

func (s *State) RecordSuccess(commit, image, composeHash string, keep int) {
	s.clearFailedDeploy()
	s.touch()
	s.LastAttemptedCommit = commit
	s.LastDeployedCommit = commit
	s.LastSuccessfulImage = image
	s.LastComposeHash = composeHash
	s.LastStatus = "success"
	s.LastError = ""
	s.LastErrorAt = time.Time{}
	s.Builds = append([]Build{{Commit: commit, Image: image, Deployed: time.Now().UTC()}}, s.Builds...)
	seen := map[string]bool{}
	unique := make([]Build, 0, len(s.Builds))
	for _, build := range s.Builds {
		if build.Commit == "" || seen[build.Commit] {
			continue
		}
		seen[build.Commit] = true
		unique = append(unique, build)
	}
	if keep > 0 && len(unique) > keep {
		unique = unique[:keep]
	}
	s.Builds = unique
}
