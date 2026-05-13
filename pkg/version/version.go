/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
