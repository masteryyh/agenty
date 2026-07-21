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
