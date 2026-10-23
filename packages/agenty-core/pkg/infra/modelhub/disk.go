package modelhub

import (
	"fmt"
	"os"
	"path/filepath"
)

type Storage struct {
	Directory      string `json:"directory"`
	Volume         string `json:"volume"`
	TotalBytes     uint64 `json:"totalBytes"`
	UsedBytes      uint64 `json:"usedBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
}

// Inspect the nearest existing parent so merely browsing does not create directories.
func diskSpace(directory string) (Storage, error) {
	parent := directory
	for {
		info, err := os.Stat(parent)
		if err == nil {
			if !info.IsDir() {
				return Storage{}, fmt.Errorf("model storage parent is not a directory: %s", parent)
			}
			break
		}
		if !os.IsNotExist(err) {
			return Storage{}, err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return Storage{}, err
		}
		parent = next
	}
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return Storage{}, err
	}
	storage, err := platformSpace(resolved)
	storage.Directory = directory
	return storage, err
}
