package pause

import (
	"errors"
	"os"
	"path/filepath"
)

func Path(dataDir string) string {
	return filepath.Join(dataDir, "paused")
}

func IsPaused(dataDir string) (bool, error) {
	_, err := os.Stat(Path(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func Set(dataDir string, paused bool) error {
	path := Path(dataDir)
	if !paused {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("automatic deployments paused\n"), 0o644)
}
