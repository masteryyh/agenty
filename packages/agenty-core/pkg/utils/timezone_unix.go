//go:build darwin || linux

package utils

import (
	"os"
	"strings"
)

func platformTimezoneName() (string, bool) {
	for _, path := range []string{"/etc/localtime", "/var/db/timezone/zoneinfo/localtime"} {
		target, err := os.Readlink(path)
		if err != nil {
			continue
		}
		if name := normalizeTimezone(target); name != "" && (name == "UTC" || strings.Contains(name, "/")) {
			return name, true
		}
	}

	data, err := os.ReadFile("/etc/timezone")
	if err == nil {
		if name := normalizeTimezone(string(data)); name != "" {
			return name, true
		}
	}

	return "", false
}
