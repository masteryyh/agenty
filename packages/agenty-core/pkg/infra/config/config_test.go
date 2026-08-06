package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitializeDataDirCreatesStructure(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGENTY_DATA_DIR", tmpDir)

	if err := InitializeDataDir(); err != nil {
		t.Fatalf("InitializeDataDir: %v", err)
	}

	paths, err := ResolvePaths()
	if err != nil {
		t.Errorf("ResolvePaths: %v", err)
	}

	for _, dir := range []string{paths.SessionsDir, paths.AgentsDir, paths.ProvidersDir} {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("expected directory %s to exist", dir)
		}
	}

	if _, err := os.Stat(paths.ConfigFile); os.IsNotExist(err) {
		t.Errorf("expected config file %s to exist", paths.ConfigFile)
	}
}

func TestInitializeDataDirIsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGENTY_DATA_DIR", tmpDir)

	if err := InitializeDataDir(); err != nil {
		t.Fatalf("first init: %v", err)
	}
	if err := InitializeDataDir(); err != nil {
		t.Fatalf("second init: %v", err)
	}

	// Config should still be readable.
	cfg, paths, err := Load()
	if err != nil {
		t.Fatalf("Load after double init: %v", err)
	}
	if cfg.Version != 1 {
		t.Errorf("config version = %d, want 1", cfg.Version)
	}
	if paths.DataDir != tmpDir {
		t.Errorf("paths.DataDir = %s, want %s", paths.DataDir, tmpDir)
	}
}

func TestLoadReturnsNotFoundWhenMissing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGENTY_DATA_DIR", tmpDir)

	_, _, err := Load()
	if err != ErrConfigNotFound {
		t.Errorf("Load() = %v, want ErrConfigNotFound", err)
	}
}

func TestLoadRejectsCorruptedConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGENTY_DATA_DIR", tmpDir)

	paths, err := ResolvePaths()
	if err != nil {
		t.Errorf("ResolvePaths: %v", err)
	}

	if err := os.MkdirAll(paths.DataDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.ConfigFile, []byte("{malformed"), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err = Load()
	if err == nil {
		t.Error("expected error for malformed config, got nil")
	}
}

func TestResolvePathsUsesEnvVar(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom")
	t.Setenv("AGENTY_DATA_DIR", custom)

	paths, err := ResolvePaths()
	if err != nil {
		t.Errorf("ResolvePaths: %v", err)
	}

	if paths.DataDir != custom {
		t.Errorf("DataDir = %s, want %s", paths.DataDir, custom)
	}
	if paths.ConfigFile != filepath.Join(custom, "config.json") {
		t.Errorf("ConfigFile = %s", paths.ConfigFile)
	}
}

func TestLoadYAML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGENTY_DATA_DIR", tmpDir)

	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte("version: 7\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, paths, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Version != 7 {
		t.Errorf("version = %d, want 7", cfg.Version)
	}
	if want := filepath.Join(tmpDir, "config.yaml"); paths.ConfigFile != want {
		t.Errorf("ConfigFile = %s, want %s", paths.ConfigFile, want)
	}
}

func TestLoadTOML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGENTY_DATA_DIR", tmpDir)

	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config.toml"), []byte("version = 9\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Version != 9 {
		t.Errorf("version = %d, want 9", cfg.Version)
	}
}

func TestLoadDetectsFirstFormat(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGENTY_DATA_DIR", tmpDir)

	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte("version: 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(`{"version": 1}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, paths, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Version != 2 {
		t.Errorf("version = %d, want 2 (from yaml)", cfg.Version)
	}
	if filepath.Ext(paths.ConfigFile) != ".yaml" {
		t.Errorf("ConfigFile = %s, want .yaml", paths.ConfigFile)
	}
}

func TestInitializeDataDirPreservesExistingYAML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGENTY_DATA_DIR", tmpDir)

	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatal(err)
	}
	yamlFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(yamlFile, []byte("version: 5\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := InitializeDataDir(); err != nil {
		t.Fatalf("InitializeDataDir: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "config.json")); !os.IsNotExist(err) {
		t.Errorf("expected config.json not to be created; stat err = %v", err)
	}

	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Version != 5 {
		t.Errorf("version = %d, want 5", cfg.Version)
	}
}

func TestLoadRejectsCorruptedYAML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGENTY_DATA_DIR", tmpDir)

	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte("foo: [unclosed\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := Load()
	if err == nil {
		t.Error("expected error for malformed yaml, got nil")
	}
}

func TestInitializeDataDirRejectsCorruptedYAML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGENTY_DATA_DIR", tmpDir)

	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte("foo: [unclosed\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := InitializeDataDir(); err == nil {
		t.Error("expected error for malformed yaml during init, got nil")
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		env  map[string]string
		want LoggingConfig
	}{
		{
			name: "empty env preserves file values",
			cfg:  Config{Logging: LoggingConfig{Level: "warn", Format: "text"}},
			env:  map[string]string{},
			want: LoggingConfig{Level: "warn", Format: "text"},
		},
		{
			name: "non-empty env overrides file values",
			cfg:  Config{Logging: LoggingConfig{Level: "warn", Format: "text"}},
			env:  map[string]string{EnvLogLevel: "debug", EnvLogFormat: "jsonl"},
			want: LoggingConfig{Level: "debug", Format: "jsonl"},
		},
		{
			name: "whitespace-only env preserves file values",
			cfg:  Config{Logging: LoggingConfig{Level: "warn", Format: "text"}},
			env:  map[string]string{EnvLogLevel: "   ", EnvLogFormat: "\t"},
			want: LoggingConfig{Level: "warn", Format: "text"},
		},
		{
			name: "partial env overrides only set fields",
			cfg:  Config{Logging: LoggingConfig{Level: "warn", Format: "text"}},
			env:  map[string]string{EnvLogLevel: "error"},
			want: LoggingConfig{Level: "error", Format: "text"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			getenv := func(key string) string { return test.env[key] }
			applyEnvOverrides(&test.cfg, getenv)
			if test.cfg.Logging != test.want {
				t.Errorf("logging = %+v, want %+v", test.cfg.Logging, test.want)
			}
		})
	}
}

func TestLoadReadsLoggingFromConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(EnvDataDir, tmpDir)
	t.Setenv(EnvLogLevel, "")
	t.Setenv(EnvLogFormat, "")

	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(`{"version":1,"logging":{"level":"warn","format":"jsonl"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("level = %q, want warn (from file)", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "jsonl" {
		t.Errorf("format = %q, want jsonl (from file)", cfg.Logging.Format)
	}
}

func TestLoadAppliesEnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(EnvDataDir, tmpDir)
	t.Setenv(EnvLogLevel, "debug")
	t.Setenv(EnvLogFormat, "jsonl")

	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(`{"version":1,"logging":{"level":"warn","format":"text"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("level = %q, want debug (env override)", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "jsonl" {
		t.Errorf("format = %q, want jsonl (env override)", cfg.Logging.Format)
	}
}

func TestInitializeDataDirWritesLoggingDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(EnvDataDir, tmpDir)
	t.Setenv(EnvLogLevel, "")
	t.Setenv(EnvLogFormat, "")

	if err := InitializeDataDir(); err != nil {
		t.Fatalf("InitializeDataDir: %v", err)
	}

	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("default level = %q, want info", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("default format = %q, want text", cfg.Logging.Format)
	}
}

func TestInitSingleton(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(EnvDataDir, tmpDir)
	t.Setenv(EnvLogLevel, "debug")
	t.Setenv(EnvLogFormat, "jsonl")
	ResetForTesting()

	mgr, err := Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if mgr != Get() {
		t.Error("Get did not return the same instance as Init")
	}
	if got := mgr.Paths().DataDir; got != tmpDir {
		t.Errorf("DataDir = %q, want %q", got, tmpDir)
	}
	if got := mgr.Config().Logging.Level; got != "debug" {
		t.Errorf("level = %q, want debug", got)
	}
	if got := mgr.Config().Logging.Format; got != "jsonl" {
		t.Errorf("format = %q, want jsonl", got)
	}

	// A second Init returns the cached singleton without re-reading env.
	t.Setenv(EnvLogLevel, "error")
	mgr2, err := Init()
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}
	if mgr2 != mgr {
		t.Error("second Init did not return the cached singleton")
	}
	if got := mgr2.Config().Logging.Level; got != "debug" {
		t.Errorf("level = %q, want debug (cached, not re-read)", got)
	}
}
