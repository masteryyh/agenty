package version

import (
	"runtime/debug"
	"strings"
	"sync"
)

const DefaultVersion = "dev"

var Version = DefaultVersion

var (
	currentVersion string
	versionOnce    sync.Once
)

func Current() string {
	versionOnce.Do(func() {
		currentVersion = resolveVersion()
	})
	return currentVersion
}

func resolveVersion() string {
	version := strings.TrimSpace(Version)
	if version != "" && version != DefaultVersion {
		return version
	}

	if version := versionFromBuildInfo(); version != "" {
		return version
	}

	if version == "" {
		return DefaultVersion
	}
	return version
}

func versionFromBuildInfo() string {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	var revision string
	dirty := false
	for _, setting := range buildInfo.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = strings.TrimSpace(setting.Value)
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}

	if revision == "" {
		return ""
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if dirty {
		return revision + "-dirty"
	}
	return revision
}
