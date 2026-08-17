package utils

import "testing"

func TestNormalizeTimezone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "iana", value: "Asia/Shanghai", want: "Asia/Shanghai"},
		{name: "posix prefix", value: ":America/New_York", want: "America/New_York"},
		{name: "zoneinfo path", value: "/usr/share/zoneinfo/Europe/Berlin", want: "Europe/Berlin"},
		{name: "windows name", value: "China Standard Time", want: "China Standard Time"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeTimezone(tt.value); got != tt.want {
				t.Errorf("normalizeTimezone(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestTimezoneNameHasFallback(t *testing.T) {
	t.Parallel()

	if got := timezoneNameWith("", func() (string, bool) {
		return "", false
	}); got != "UTC" {
		t.Fatalf("timezoneNameWith unavailable platform = %q, want UTC", got)
	}
	if got := timezoneNameWith("Asia/Shanghai", func() (string, bool) {
		return "America/New_York", true
	}); got != "Asia/Shanghai" {
		t.Fatalf("timezoneNameWith environment = %q, want Asia/Shanghai", got)
	}
	if got := TimezoneName(); got == "" {
		t.Fatal("TimezoneName returned an empty value")
	}
}
