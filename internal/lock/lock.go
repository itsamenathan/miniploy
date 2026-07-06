package lock

import (
	"errors"
	"fmt"
	"os"
)

type Lock struct {
	path string
}

func Acquire(path string) (*Lock, error) {
	if err := os.Mkdir(path, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("deploy already in progress: %s", path)
		}
		return nil, err
	}
	return &Lock{path: path}, nil
}

func (l *Lock) Release() error {
	if l == nil {
		return nil
	}
	return os.Remove(l.path)
}
