//go:build e2e

package e2e_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
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
	name := "agenty-core"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	coreBinary = filepath.Join(buildDir, name)

	args := []string{"build"}
	if raceEnabled {
		args = append(args, "-race")
	}
	args = append(args, "-o", coreBinary, "./cmd")
	cmd := exec.Command("go", args...)
	cmd.Dir = moduleRoot
	if output, buildErr := cmd.CombinedOutput(); buildErr != nil {
		fmt.Fprintf(os.Stderr, "e2e: build agenty-core: %v\n%s", buildErr, output)
		_ = os.RemoveAll(buildDir)
		os.Exit(1)
	}

	code := m.Run()
	if removeErr := os.RemoveAll(buildDir); removeErr != nil && code == 0 {
		fmt.Fprintln(os.Stderr, "e2e: remove build directory:", removeErr)
		code = 1
	}
	os.Exit(code)
}
