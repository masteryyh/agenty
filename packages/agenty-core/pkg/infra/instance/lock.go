package instance

import (
	"fmt"
	"os"
)

func Acquire(dataDir string) (*Lock, error) {
	runtimeDir := RuntimeDir(dataDir)
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return nil, fmt.Errorf("create runtime directory: %w", err)
	}

	file, err := os.OpenFile(runtimeDir+string(os.PathSeparator)+"core.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open core lock: %w", err)
	}
	if err := lockFile(file); err != nil {
		file.Close()
		return nil, err
	}
	return &Lock{file: file}, nil
}

func (lock *Lock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockFile(lock.file)
	err := lock.file.Close()
	lock.file = nil
	return err
}
