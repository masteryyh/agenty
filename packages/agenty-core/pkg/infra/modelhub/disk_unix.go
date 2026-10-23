//go:build darwin || linux

package modelhub

import (
	"path/filepath"

	"golang.org/x/sys/unix"
)

func platformSpace(directory string) (Storage, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(directory, &stat); err != nil {
		return Storage{}, err
	}
	volume := directory
	var device unix.Stat_t
	if err := unix.Stat(directory, &device); err != nil {
		return Storage{}, err
	}
	for {
		parent := filepath.Dir(volume)
		if parent == volume {
			break
		}
		var other unix.Stat_t
		if err := unix.Stat(parent, &other); err != nil {
			return Storage{}, err
		}
		if other.Dev != device.Dev {
			break
		}
		volume = parent
	}
	block := uint64(stat.Bsize)
	return Storage{Volume: volume, TotalBytes: stat.Blocks * block, UsedBytes: (stat.Blocks - stat.Bfree) * block, AvailableBytes: uint64(max(int64(stat.Bavail), 0)) * block}, nil
}
