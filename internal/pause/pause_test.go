package pause

import "testing"

func TestPausePersistsUntilResumed(t *testing.T) {
	dir := t.TempDir()
	paused, err := IsPaused(dir)
	if err != nil || paused {
		t.Fatalf("initial IsPaused() = %t, %v", paused, err)
	}
	if err := Set(dir, true); err != nil {
		t.Fatal(err)
	}
	paused, err = IsPaused(dir)
	if err != nil || !paused {
		t.Fatalf("after Set(true), IsPaused() = %t, %v", paused, err)
	}
	if err := Set(dir, false); err != nil {
		t.Fatal(err)
	}
	paused, err = IsPaused(dir)
	if err != nil || paused {
		t.Fatalf("after Set(false), IsPaused() = %t, %v", paused, err)
	}
}
