package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/masteryyh/agenty-core/pkg/domain/localinference"
)

const (
	EnvLlamaCppWorker = "AGENTY_LLAMA_CPP_WORKER_BIN"
	EnvMLXWorker      = "AGENTY_MLX_WORKER_BIN"
)

// Worker paths belong to this core process, while user configuration remains portable.
func (m *Manager) loadWorkerOverrides(getenv func(string) string) error {
	m.workerOverrides = make(map[localinference.Backend]string)
	for _, entry := range []struct {
		name    string
		backend localinference.Backend
	}{
		{EnvLlamaCppWorker, localinference.LlamaCpp}, {EnvMLXWorker, localinference.MLX},
	} {
		command := getenv(entry.name)
		if command == "" {
			continue
		}
		if !filepath.IsAbs(command) {
			return fmt.Errorf("%s requires an absolute executable path", entry.name)
		}
		info, err := os.Stat(command)
		if err != nil {
			return fmt.Errorf("%s: %w", entry.name, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s requires a regular executable file", entry.name)
		}
		m.workerOverrides[entry.backend] = command
	}
	if len(m.workerOverrides) == 0 {
		return nil
	}
	m.persistedWorkers = slices.Clone(m.cfg.LocalInference.Workers)
	m.cfg.LocalInference = m.withWorkerOverrides(m.cfg.LocalInference)
	return m.cfg.LocalInference.Validate()
}

func (m *Manager) withWorkerOverrides(value localinference.Config) localinference.Config {
	value.Workers = slices.Clone(value.Workers)
	for _, backend := range []localinference.Backend{localinference.LlamaCpp, localinference.MLX} {
		command, ok := m.workerOverrides[backend]
		if !ok {
			continue
		}
		index := slices.IndexFunc(value.Workers, func(worker localinference.Worker) bool { return worker.Backend == backend })
		if index == -1 {
			id := "llama-cpp"
			if backend == localinference.MLX {
				id = "mlx"
			}
			value.Workers = append(value.Workers, localinference.Worker{ID: id, Backend: backend, Enabled: true})
			index = len(value.Workers) - 1
		}
		value.Workers[index].Command = command
		value.Workers[index].Arguments = nil
		value.Workers[index].Version = "dev"
	}
	return value
}

func (m *Manager) persistableLocalInference(value localinference.Config) localinference.Config {
	persisted := localinference.Config{Enabled: value.Enabled}
	for _, worker := range value.Workers {
		command, overridden := m.workerOverrides[worker.Backend]
		if !overridden || worker.Command != command {
			persisted.Workers = append(persisted.Workers, worker)
			continue
		}
		// Saving UI switches must retain the installed worker's executable and arguments.
		for _, previous := range m.persistedWorkers {
			if previous.Backend == worker.Backend {
				previous.Enabled = worker.Enabled
				persisted.Workers = append(persisted.Workers, previous)
				break
			}
		}
	}
	return persisted
}
