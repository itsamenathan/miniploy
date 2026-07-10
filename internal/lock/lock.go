package lock

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

type Lock struct {
	file *os.File
}

// Acquire obtains a non-blocking advisory lock. The operating system releases
// it automatically if the process exits unexpectedly, so a crashed deployer
// cannot leave future deployments permanently blocked.
func Acquire(path string) (*Lock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if errors.Is(err, syscall.EISDIR) {
		// A prior miniploy version created a lock directory. Lock it directly so
		// upgrades recover without manual cleanup.
		file, err = os.Open(path)
	}
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, fmt.Errorf("deploy already in progress: %s", path)
		}
		return nil, err
	}
	return &Lock{file: file}, nil
}

func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	return errors.Join(unlockErr, closeErr)
}
