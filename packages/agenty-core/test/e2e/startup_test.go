//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStartupRejectsMalformedExistingConfig(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "config.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), processTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, coreBinary)
	cmd.Dir = moduleRoot
	cmd.Env = replaceEnv(os.Environ(), "AGENTY_DATA_DIR", dataDir)
	cmd.Stdin = strings.NewReader("")
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("startup error = %v, want exit code 1", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("startup stdout = %q, want empty", stdout.String())
	}
	if diagnostics := stderr.String(); !strings.Contains(diagnostics, "agenty-core: failed to open repositories:") || !strings.Contains(diagnostics, "existing config file is malformed") {
		t.Fatalf("startup stderr = %q", diagnostics)
	}
}
