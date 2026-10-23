package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty-core/pkg/domain/localinference"
)

func TestWorkerOverridesRemainProcessScopedAcrossConfigSaves(t *testing.T) {
	directory := t.TempDir()
	command := filepath.Join(directory, "development worker")
	if err := os.WriteFile(command, []byte("worker"), 0o755); err != nil {
		t.Fatal(err)
	}
	installed := localinference.Worker{ID: "native", Backend: localinference.LlamaCpp, Enabled: false, Command: filepath.Join(directory, "installed"), Arguments: []string{"--installed"}, Version: "1.0"}
	manager := &Manager{cfg: &Config{LocalInference: localinference.Config{Workers: []localinference.Worker{installed}}}, paths: &Paths{ConfigFile: filepath.Join(directory, "config.json")}}
	if err := os.WriteFile(manager.paths.ConfigFile, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.loadWorkerOverrides(func(name string) string {
		if name == EnvLlamaCppWorker || name == EnvMLXWorker {
			return command
		}
		return ""
	}); err != nil {
		t.Fatal(err)
	}
	value := manager.LocalInference()
	if value.Enabled || len(value.Workers) != 2 || value.Workers[0].ID != "native" || value.Workers[0].Enabled || value.Workers[0].Command != command || len(value.Workers[0].Arguments) != 0 || !value.Workers[1].Enabled {
		t.Fatalf("unexpected runtime overrides: %+v", value)
	}
	value.Enabled = true
	value.Workers[0].Enabled = true
	value.Workers[1].Enabled = false
	if err := manager.SetLocalInference(value); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(manager.paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	var durable Config
	if err := json.Unmarshal(saved, &durable); err != nil {
		t.Fatal(err)
	}
	installed.Enabled = true
	if !durable.LocalInference.Enabled || !reflect.DeepEqual(durable.LocalInference.Workers, []localinference.Worker{installed}) {
		t.Fatalf("development paths leaked to disk: %s", saved)
	}
	effective := manager.LocalInference()
	if effective.Workers[0].Command != command || effective.Workers[1].Enabled {
		t.Fatalf("runtime overrides lost after save: %+v", effective)
	}
	// A replacement installation is saved, while the startup override still wins for this process.
	effective.Workers[0].Command = filepath.Join(directory, "new installation")
	effective.Workers[0].Version = "2.0"
	if err := manager.SetLocalInference(effective); err != nil {
		t.Fatal(err)
	}
	if manager.LocalInference().Workers[0].Command != command || manager.persistedWorkers[0].Command != effective.Workers[0].Command {
		t.Fatal("replacement installation or startup override was lost")
	}
}

func TestWorkerOverridesValidatePathsAndPreserveDefaults(t *testing.T) {
	for _, command := range []string{"relative", t.TempDir(), filepath.Join(t.TempDir(), "missing")} {
		t.Run(command, func(t *testing.T) {
			manager := &Manager{cfg: &Config{}}
			if err := manager.loadWorkerOverrides(func(name string) string {
				if name == EnvLlamaCppWorker {
					return command
				}
				return ""
			}); err == nil {
				t.Fatal("invalid executable accepted")
			}
		})
	}
	manager := &Manager{cfg: &Config{}}
	if err := manager.loadWorkerOverrides(func(string) string { return "" }); err != nil {
		t.Fatal(err)
	}
	if len(manager.LocalInference().Workers) != 0 || manager.LocalInference().Enabled {
		t.Fatal("default local inference changed")
	}
}
