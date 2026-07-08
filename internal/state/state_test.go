package state

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestRecordFailureStoresError(t *testing.T) {
	var st State
	st.RecordFailure("abc123", errors.New("build failed"))

	if st.LastStatus != "failed" {
		t.Fatalf("LastStatus = %q, want failed", st.LastStatus)
	}
	if st.LastAttemptedCommit != "abc123" {
		t.Fatalf("LastAttemptedCommit = %q, want abc123", st.LastAttemptedCommit)
	}
	if st.LastError != "build failed" {
		t.Fatalf("LastError = %q, want build failed", st.LastError)
	}
	if st.LastErrorAt.IsZero() {
		t.Fatal("LastErrorAt is zero, want timestamp")
	}
}

func TestRecordSuccessClearsError(t *testing.T) {
	var st State
	st.RecordFailure("abc123", errors.New("build failed"))
	st.RecordSuccess("abc123", "app:abc123", "deadbeef", 3)

	if st.LastStatus != "success" {
		t.Fatalf("LastStatus = %q, want success", st.LastStatus)
	}
	if st.LastError != "" {
		t.Fatalf("LastError = %q, want empty", st.LastError)
	}
	if !st.LastErrorAt.IsZero() {
		t.Fatalf("LastErrorAt = %v, want zero", st.LastErrorAt)
	}
}

func TestRecordSuccessKeepsUniqueRecentBuilds(t *testing.T) {
	var st State
	st.RecordSuccess("one", "app:one", "hash-one", 3)
	st.RecordSuccess("two", "app:two", "hash-two", 3)
	st.RecordSuccess("one", "app:one-new", "hash-one-new", 3)
	st.RecordSuccess("three", "app:three", "hash-three", 3)
	st.RecordSuccess("four", "app:four", "hash-four", 3)

	if len(st.Builds) != 3 {
		t.Fatalf("len(Builds) = %d, want 3", len(st.Builds))
	}
	wantCommits := []string{"four", "three", "one"}
	for i, want := range wantCommits {
		if st.Builds[i].Commit != want {
			t.Fatalf("Builds[%d].Commit = %q, want %q", i, st.Builds[i].Commit, want)
		}
	}
	if st.Builds[2].Image != "app:one-new" {
		t.Fatalf("deduplicated image = %q, want app:one-new", st.Builds[2].Image)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	original := State{LastDeployedCommit: "abc123", LastStatus: "success"}

	if err := Save(path, original); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.LastDeployedCommit != original.LastDeployedCommit {
		t.Fatalf("LastDeployedCommit = %q, want %q", loaded.LastDeployedCommit, original.LastDeployedCommit)
	}
	if loaded.Updated.IsZero() {
		t.Fatal("Updated is zero, want timestamp")
	}
}

func TestLoadMissingState(t *testing.T) {
	loaded, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.LastStatus != "" || loaded.LastDeployedCommit != "" || len(loaded.Builds) != 0 {
		t.Fatalf("Load() = %+v, want empty state", loaded)
	}
}

func TestRecordRedeploySuccessClearsError(t *testing.T) {
	var st State
	st.RecordFailure("abc123", errors.New("compose failed"))
	st.RecordRedeploySuccess("abc123", "deadbeef")

	if st.LastStatus != "success" {
		t.Fatalf("LastStatus = %q, want success", st.LastStatus)
	}
	if st.LastError != "" {
		t.Fatalf("LastError = %q, want empty", st.LastError)
	}
	if !st.LastErrorAt.IsZero() {
		t.Fatalf("LastErrorAt = %v, want zero", st.LastErrorAt)
	}
}
