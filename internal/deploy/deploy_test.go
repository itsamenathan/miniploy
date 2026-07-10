package deploy

import (
	"errors"
	"testing"

	"github.com/itsamenathan/miniploy/internal/state"
)

func TestRecordFailureReturnsPersistenceError(t *testing.T) {
	failure := errors.New("build failed")
	persistenceErr := errors.New("disk full")
	runner := Runner{stateSaver: func(state.State) error { return persistenceErr }}

	err := runner.recordFailure(state.State{}, "abc123", failure)
	if !errors.Is(err, failure) {
		t.Fatalf("recordFailure() error = %v, want original failure", err)
	}
	if !errors.Is(err, persistenceErr) {
		t.Fatalf("recordFailure() error = %v, want persistence failure", err)
	}
}
