package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

type Manager struct {
	mu    sync.RWMutex
	cfg   *Config
	paths *Paths
}

func (m *Manager) Config() *Config { return m.cfg }

func (m *Manager) Paths() *Paths { return m.paths }

func (m *Manager) Initialized() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.cfg.Initialized
}

func (m *Manager) SetInitialized(initialized bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cfg.Initialized == initialized {
		return nil
	}
	if err := persistConfigValue(m.paths.ConfigFile, "initialized", initialized); err != nil {
		return fmt.Errorf("config: persist initialized: %w", err)
	}
	m.cfg.Initialized = initialized
	return nil
}

func persistConfigValue(file, key string, value any) error {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(file)), ".")
	if ext == "yml" {
		ext = "yaml"
	}
	if ext != "json" && ext != "yaml" && ext != "toml" {
		return fmt.Errorf("unsupported config format %q", ext)
	}

	v := viper.New()
	v.SetConfigType(ext)
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	if err := v.ReadConfig(f); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	v.Set(key, value)

	tmp, err := os.CreateTemp(filepath.Dir(file), ".config-*"+filepath.Ext(file))
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	defer os.Remove(tmpName)

	if err := v.WriteConfigAs(tmpName); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, file)
}

var (
	initOnce sync.Once
	mgr      *Manager
	initErr  error
)

func Init() (*Manager, error) {
	initOnce.Do(func() {
		if err := InitializeDataDir(); err != nil {
			initErr = err
			return
		}

		cfg, paths, err := Load()
		if err != nil {
			initErr = err
			return
		}
		mgr = &Manager{cfg: cfg, paths: paths}
	})
	return mgr, initErr
}

func Get() *Manager {
	if mgr == nil {
		panic("config: manager not initialized, call Init first")
	}
	return mgr
}

func ResetForTesting() {
	initOnce = sync.Once{}
	mgr = nil
	initErr = nil
}
