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

package utils

import (
	"errors"
	"path/filepath"
	"strings"
)

func GetCleanPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("path cannot be empty")
	}

	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	return abs, nil
}

func PathContained(basePaths []string, targetPath string) (bool, error) {
	if len(basePaths) == 0 {
		return true, nil
	}

	for _, base := range basePaths {
		rel, err := filepath.Rel(base, targetPath)
		if err != nil {
			return false, err
		}

		if !strings.HasPrefix(rel, "..") {
			return true, nil
		}
	}
	return false, nil
}
