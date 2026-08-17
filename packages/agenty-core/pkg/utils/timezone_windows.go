//go:build windows

package utils

import "golang.org/x/sys/windows/registry"

func platformTimezoneName() (string, bool) {
	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\TimeZoneInformation`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return "", false
	}
	defer key.Close()

	for _, valueName := range []string{"TimeZoneKeyName", "StandardName"} {
		name, _, err := key.GetStringValue(valueName)
		if err == nil && name != "" {
			return name, true
		}
	}
	return "", false
}
