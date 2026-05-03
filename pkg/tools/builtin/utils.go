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

package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/masteryyh/agenty/pkg/consts"
)

const fileLockRetryDelay = 25 * time.Millisecond

func fileLockPath(path string) string {
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".lock")
}

func acquireFileLock(ctx context.Context, path string, exclusive bool) (*flock.Flock, error) {
	fileLock := flock.New(fileLockPath(path))

	var (
		locked bool
		err    error
	)
	if exclusive {
		locked, err = fileLock.TryLockContext(ctx, fileLockRetryDelay)
	} else {
		locked, err = fileLock.TryRLockContext(ctx, fileLockRetryDelay)
	}
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, fmt.Errorf("file lock was not acquired")
	}

	return fileLock, nil
}

func releaseFileLock(fileLock *flock.Flock) error {
	if fileLock == nil {
		return nil
	}
	return fileLock.Unlock()
}

func validatePath(path string) error {
	cleanPath := filepath.Clean(path)
	if _, ok := consts.BlockingPaths[cleanPath]; ok {
		return fmt.Errorf("path %s is not allowed because it may block the process; use run_shell_command if you need to operate on it", path)
	}

	lowerPath := strings.ToLower(cleanPath)
	for _, prefix := range consts.SensitiveFileToolPathPrefixes {
		if isPathUnderPrefix(lowerPath, prefix) {
			return fmt.Errorf("path %s is a sensitive system path; use run_shell_command if you need to operate on it", path)
		}
	}

	return nil
}

func isPathUnderPrefix(path, prefix string) bool {
	normalizedPath := strings.ReplaceAll(path, "\\", "/")
	normalizedPrefix := strings.ReplaceAll(prefix, "\\", "/")
	return normalizedPath == normalizedPrefix || strings.HasPrefix(normalizedPath, normalizedPrefix+"/")
}

func validateExistingFile(path string) error {
	for current := path; ; current = filepath.Dir(current) {
		target, err := filepath.EvalSymlinks(current)
		if err == nil {
			return validatePath(target)
		}
		if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
	}
}
