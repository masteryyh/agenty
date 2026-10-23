package localinference

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Backend string

const (
	MLX      Backend = "mlx"
	LlamaCpp Backend = "llamaCpp"
)

func (b Backend) Valid() bool { return b == MLX || b == LlamaCpp }

type Worker struct {
	ID        string   `json:"id" mapstructure:"id"`
	Backend   Backend  `json:"backend" mapstructure:"backend"`
	Enabled   bool     `json:"enabled" mapstructure:"enabled"`
	Command   string   `json:"command" mapstructure:"command"`
	Arguments []string `json:"arguments,omitempty" mapstructure:"arguments"`
	Version   string   `json:"version,omitempty" mapstructure:"version"`
}
type Config struct {
	Enabled bool     `json:"enabled" mapstructure:"enabled"`
	Workers []Worker `json:"workers" mapstructure:"workers"`
}

func (c Config) Validate() error {
	seen := make(map[string]bool)
	backends := make(map[Backend]bool)
	for _, w := range c.Workers {
		if w.ID == "" || filepath.Base(w.ID) != w.ID || strings.ContainsAny(w.ID, "/\\.") || !w.Backend.Valid() || !filepath.IsAbs(w.Command) {
			return fmt.Errorf("worker requires a simple ID, supported backend and absolute executable path")
		}
		if seen[w.ID] || backends[w.Backend] {
			return fmt.Errorf("only one worker per ID and backend is supported")
		}
		seen[w.ID], backends[w.Backend] = true, true
	}
	return nil
}

type Resource struct {
	ID              string  `json:"resourceId"`
	Backend         Backend `json:"backend"`
	Format          string  `json:"format"`
	WeightsPath     string  `json:"weightsPath"`
	CodecDirectory  string  `json:"codecDirectory"`
	TokenizerSHA256 string  `json:"tokenizerSha256,omitempty"`
}
type Model struct {
	BackendPreference string     `json:"backendPreference"`
	OutputParser      string     `json:"outputParser"`
	TemplateName      string     `json:"templateName,omitempty"`
	ThinkingEffort    string     `json:"thinkingEffort,omitempty"`
	Resources         []Resource `json:"resources"`
}

func (m *Model) Validate() error {
	if m == nil || len(m.Resources) == 0 {
		return fmt.Errorf("local model needs at least one resource")
	}
	if m.OutputParser != "qwen3" && m.OutputParser != "qwen3_5" {
		return fmt.Errorf("supported output parsers: qwen3, qwen3_5")
	}
	if m.BackendPreference != "" && m.BackendPreference != "auto" && !Backend(m.BackendPreference).Valid() {
		return fmt.Errorf("invalid backend preference")
	}
	if m.ThinkingEffort != "" && m.ThinkingEffort != "medium" {
		return fmt.Errorf("local thinking currently maps medium to enable_thinking")
	}
	seen := make(map[string]bool)
	for _, r := range m.Resources {
		if r.ID == "" || seen[r.ID] || !r.Backend.Valid() || !filepath.IsAbs(r.WeightsPath) || !filepath.IsAbs(r.CodecDirectory) {
			return fmt.Errorf("resource needs a unique ID, supported backend and absolute model/codec paths")
		}
		if (r.Backend == MLX && r.Format != "safetensors") || (r.Backend == LlamaCpp && r.Format != "gguf") {
			return fmt.Errorf("resource format does not match backend")
		}
		seen[r.ID] = true
	}
	return nil
}

type WorkerState struct {
	ID              string  `json:"id"`
	Backend         Backend `json:"backend"`
	Status          string  `json:"status"`
	ModelResourceID string  `json:"modelResourceId,omitempty"`
	Error           string  `json:"error,omitempty"`
	EngineVersion   string  `json:"engineVersion,omitempty"`
}
type Snapshot struct {
	Enabled bool          `json:"enabled"`
	Workers []WorkerState `json:"workers"`
}
