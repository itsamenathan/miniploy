package compose

import "testing"

func TestHashIsStableAndSensitiveToRenderedConfig(t *testing.T) {
	first := "services:\n  app:\n    image: app:one\n"
	if got := Hash(first); got != Hash(first) {
		t.Fatalf("Hash() = %q on identical input, want stable value", got)
	}
	if Hash(first) == Hash("services:\n  app:\n    image: app:two\n") {
		t.Fatal("Hash() is identical for different rendered configurations")
	}
}
