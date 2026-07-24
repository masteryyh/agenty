//go:build e2e

package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigFileDrivesLogging verifies that logging settings are read from the
// config file when no logging env is set: a jsonl config produces core.jsonl
// carrying the startup line, and no core.log is created.
func TestConfigFileDrivesLogging(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "config.json"), []byte(`{"version":1,"logging":{"level":"info","format":"jsonl"}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	core := startCoreAt(t, dataDir, coreEnv(dataDir))
	core.Close(t)

	jsonlMatches, err := filepath.Glob(filepath.Join(dataDir, "logs", "*", "*", "*", "core.jsonl"))
	if err != nil {
		t.Fatalf("glob jsonl log: %v", err)
	}
	if len(jsonlMatches) != 1 {
		t.Fatalf("jsonl log files = %v, want one", jsonlMatches)
	}
	data, err := os.ReadFile(jsonlMatches[0])
	if err != nil {
		t.Fatalf("read jsonl log: %v", err)
	}
	if !strings.Contains(string(data), "agenty-core started") {
		t.Fatalf("jsonl log = %q, want startup line driven by config file", string(data))
	}

	textMatches, err := filepath.Glob(filepath.Join(dataDir, "logs", "*", "*", "*", "core.log"))
	if err != nil {
		t.Fatalf("glob text log: %v", err)
	}
	if len(textMatches) != 0 {
		t.Fatalf("text log files = %v, want none when format=jsonl", textMatches)
	}
}

// TestEnvOverridesConfigFile verifies that a non-empty logging env variable
// takes precedence over the config file: config says text, env says jsonl, so
// core.jsonl is produced.
func TestEnvOverridesConfigFile(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "config.json"), []byte(`{"version":1,"logging":{"level":"info","format":"text"}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	env := replaceEnv(coreEnv(dataDir), "AGENTY_LOG_FORMAT", "jsonl")

	core := startCoreAt(t, dataDir, env)
	core.Close(t)

	jsonlMatches, err := filepath.Glob(filepath.Join(dataDir, "logs", "*", "*", "*", "core.jsonl"))
	if err != nil {
		t.Fatalf("glob jsonl log: %v", err)
	}
	if len(jsonlMatches) != 1 {
		t.Fatalf("jsonl log files = %v, want one (env overrode config file text)", jsonlMatches)
	}

	textMatches, err := filepath.Glob(filepath.Join(dataDir, "logs", "*", "*", "*", "core.log"))
	if err != nil {
		t.Fatalf("glob text log: %v", err)
	}
	if len(textMatches) != 0 {
		t.Fatalf("text log files = %v, want none (env overrode to jsonl)", textMatches)
	}
}

// TestConfigFileLevelFiltersStartupLog verifies that the config file level is
// honored: level=error filters out the INFO startup line, so core.log exists
// but does not contain "agenty-core started".
func TestConfigFileLevelFiltersStartupLog(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "config.json"), []byte(`{"version":1,"logging":{"level":"error","format":"text"}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	core := startCoreAt(t, dataDir, coreEnv(dataDir))
	core.Close(t)

	matches, err := filepath.Glob(filepath.Join(dataDir, "logs", "*", "*", "*", "core.log"))
	if err != nil {
		t.Fatalf("glob log: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("log files = %v, want one", matches)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if strings.Contains(string(data), "agenty-core started") {
		t.Fatalf("log = %q, want startup INFO line filtered out by level=error", string(data))
	}
}
