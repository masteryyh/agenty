//go:build e2e

package e2e_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

var (
	coreBinary string
	moduleRoot string
)

func TestMain(m *testing.M) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "e2e: resolve test source location")
		os.Exit(1)
	}
	moduleRoot = filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))

	buildDir, err := os.MkdirTemp("", "agenty-core-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: create build directory:", err)
		os.Exit(1)
	}
	coreBinary = filepath.Join(buildDir, executableName("agenty-core"))

	args := []string{"build"}
	if raceEnabled {
		args = append(args, "-race")
	}
	args = append(
		args,
		"-o",
		coreBinary,
		"./cmd",
	)
	cmd := exec.Command("go", args...)
	cmd.Dir = moduleRoot
	if output, buildErr := cmd.CombinedOutput(); buildErr != nil {
		fmt.Fprintf(
			os.Stderr,
			"e2e: build agenty-core: %v\n%s",
			buildErr,
			output,
		)
		_ = os.RemoveAll(buildDir)
		os.Exit(1)
	}

	code := m.Run()
	if attempts, firstErr, removeErr := removeBuildDir(buildDir); removeErr != nil {
		fmt.Fprintf(os.Stderr, "e2e: remove build directory after %d attempts: %v (first error: %v)\n", attempts, removeErr, firstErr)
		if code == 0 {
			code = 1
		}
	} else if attempts > 1 {
		fmt.Fprintf(os.Stderr, "e2e: build directory cleanup succeeded after %d attempts; initial removal error: %v\n", attempts, firstErr)
	}
	os.Exit(code)
}

func removeBuildDir(path string) (int, error, error) {
	const maxAttempts = 150
	var firstErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := os.RemoveAll(path)
		if err == nil {
			return attempt, firstErr, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		if runtime.GOOS != "windows" || attempt == maxAttempts {
			return attempt, firstErr, err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return maxAttempts, firstErr, fmt.Errorf("build directory cleanup exhausted %d attempts", maxAttempts)
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
