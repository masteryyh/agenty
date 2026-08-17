//go:build !darwin && !linux && !windows

package utils

func platformTimezoneName() (string, bool) {
	return "", false
}
