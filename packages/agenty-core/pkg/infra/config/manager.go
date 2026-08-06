package config

import "sync"

type Manager struct {
	cfg   *Config
	paths *Paths
}

func (m *Manager) Config() *Config { return m.cfg }

func (m *Manager) Paths() *Paths { return m.paths }

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
