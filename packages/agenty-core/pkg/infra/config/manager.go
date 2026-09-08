package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
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

func (m *Manager) DefaultModel() (shared.ModelRef, shared.ReasoningEffort) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.cfg.DefaultProviderCode == "" || m.cfg.DefaultModelCode == "" {
		return shared.ModelRef{}, shared.ReasoningOff
	}
	effort := shared.ReasoningEffort(m.cfg.DefaultReasoningEffort)
	if effort == "" {
		effort = shared.ReasoningOff
	}
	provider, err := shared.NewCode(m.cfg.DefaultProviderCode)
	if err != nil {
		return shared.ModelRef{}, effort
	}
	model, err := shared.NewModelCode(m.cfg.DefaultModelCode)
	if err != nil {
		return shared.ModelRef{}, effort
	}
	return shared.NewModelRef(provider, model), effort
}

func (m *Manager) SetDefaultModel(model shared.ModelRef, effort shared.ReasoningEffort) error {
	if model.IsZero() {
		return fmt.Errorf("config: default model is empty")
	}
	if !effort.Valid() {
		return fmt.Errorf("config: invalid default reasoning effort %q", effort)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := persistConfigValues(m.paths.ConfigFile, map[string]any{
		"defaultProviderCode":    model.ProviderCode.String(),
		"defaultModelCode":       model.ModelCode.String(),
		"defaultReasoningEffort": string(effort),
	}); err != nil {
		return fmt.Errorf("config: persist default model: %w", err)
	}
	m.cfg.DefaultProviderCode = model.ProviderCode.String()
	m.cfg.DefaultModelCode = model.ModelCode.String()
	m.cfg.DefaultReasoningEffort = string(effort)
	return nil
}

func persistConfigValue(file, key string, value any) error {
	return persistConfigValues(file, map[string]any{key: value})
}

func persistConfigValues(file string, values map[string]any) error {
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
	for key, value := range values {
		v.Set(key, value)
	}

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
