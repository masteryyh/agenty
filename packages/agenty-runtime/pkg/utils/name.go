package utils

import (
	"regexp"
	"strings"
)

var (
	nameSanitizer = regexp.MustCompile(`[^a-z0-9]+`)
)

func SanitizeName(name, defaultName string) string {
	lowered := strings.ToLower(name)
	sanitized := nameSanitizer.ReplaceAllString(lowered, "_")
	sanitized = strings.Trim(sanitized, "_")
	if sanitized == "" {
		return defaultName
	}
	return sanitized
}
