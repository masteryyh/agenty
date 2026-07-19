package utils

import (
	"errors"
	"os"
	"path/filepath"
)

func GetCleanPathWithBase(path, base string, followSymlink bool) (string, error) {
	if base != "" && !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	return GetCleanPath(path, followSymlink)
}

func GetCleanPath(path string, followSymlink bool) (string, error) {
	if path == "" {
		return "", errors.New("path cannot be empty")
	}

	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}

	if followSymlink {
		if _, err := os.Stat(abs); err != nil {
			if os.IsNotExist(err) {
				return abs, nil
			}
			return "", err
		}

		eval, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", err
		}
		return eval, nil
	}
	return abs, nil
}
