package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
	LastStatus          string    `json:"lastStatus"`
	Builds              []Build   `json:"builds"`
	Updated             time.Time `json:"updated"`
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

func Save(path string, st State) error {
	st.Updated = time.Now().UTC()
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

func (s *State) RecordAttempt(commit string) {
	s.LastAttemptedCommit = commit
	s.LastStatus = "building"
}

func (s *State) RecordFailure(commit string) {
	s.LastAttemptedCommit = commit
	s.LastStatus = "failed"
}

func (s *State) RecordSuccess(commit, image string, keep int) {
	s.LastAttemptedCommit = commit
	s.LastDeployedCommit = commit
	s.LastSuccessfulImage = image
	s.LastStatus = "success"
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
