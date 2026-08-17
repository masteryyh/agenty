package utils

import (
	"os"
	"strings"
)

func TimezoneName() string {
	return timezoneNameWith(os.Getenv("TZ"), platformTimezoneName)
}

func timezoneNameWith(environment string, platform func() (string, bool)) string {
	if name := normalizeTimezone(environment); name != "" {
		return name
	}
	if platform != nil {
		if name, ok := platform(); ok {
			if normalized := normalizeTimezone(name); normalized != "" {
				return normalized
			}
		}
	}

	return "UTC"
}

func normalizeTimezone(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, ":")
	if value == "" {
		return ""
	}

	const zoneinfoMarker = "/zoneinfo/"
	if index := strings.LastIndex(value, zoneinfoMarker); index >= 0 {
		return value[index+len(zoneinfoMarker):]
	}
	return value
}
