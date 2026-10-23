//go:build e2e

package e2e_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// These are HTTP wire fixtures; E2E must not import core implementation packages.
type localWorker struct {
	ID        string   `json:"id"`
	Backend   string   `json:"backend"`
	Enabled   bool     `json:"enabled"`
	Command   string   `json:"command"`
	Arguments []string `json:"arguments,omitempty"`
}
type localConfig struct {
	Enabled bool          `json:"enabled"`
	Workers []localWorker `json:"workers"`
}
type localSnapshot struct {
	Enabled bool `json:"enabled"`
	Workers []struct {
		Status string `json:"status"`
	} `json:"workers"`
}
type localResource struct {
	ID              string `json:"resourceId"`
	Backend         string `json:"backend"`
	Format          string `json:"format"`
	WeightsPath     string `json:"weightsPath"`
	CodecDirectory  string `json:"codecDirectory"`
	TokenizerSHA256 string `json:"tokenizerSha256,omitempty"`
}
type localModel struct {
	OutputParser   string          `json:"outputParser"`
	ThinkingEffort string          `json:"thinkingEffort,omitempty"`
	Resources      []localResource `json:"resources"`
}
type localProvider struct {
	Builtin    bool   `json:"builtin"`
	AuthMethod string `json:"authMethod"`
	Models     []struct {
		Local *localModel `json:"local"`
	} `json:"models"`
}

func TestLocalConfigurationAndModelsSurviveRestart(t *testing.T) {
	t.Parallel()
	ctx, cancel := testContext(t)
	defer cancel()
	directory := t.TempDir()
	process := startCoreAt(t, directory, coreEnv(directory))
	command, err := os.Executable()
	requireNoError(t, err)
	config := localConfig{Workers: []localWorker{{ID: "native", Backend: "llamaCpp", Command: command}}}
	requireNoError(t, process.Request(ctx, http.MethodPatch, "/v1/local-inference/config", config, nil))
	// A partial patch must preserve installed worker records.
	requireNoError(t, process.Request(ctx, http.MethodPatch, "/v1/local-inference/config", map[string]any{"enabled": true}, nil))
	var runtime localSnapshot
	requireNoError(t, process.Request(ctx, http.MethodGet, "/v1/local-inference/runtime", nil, &runtime))
	if !runtime.Enabled || len(runtime.Workers) != 1 || runtime.Workers[0].Status != "stopped" {
		t.Fatalf("disabled worker started: %+v", runtime)
	}
	codec, err := filepath.Abs("../../../llm-codec/tests/fixtures/basic")
	requireNoError(t, err)
	local := &localModel{OutputParser: "qwen3", Resources: []localResource{{ID: "gguf", Backend: "llamaCpp", Format: "gguf", WeightsPath: filepath.Join(directory, "model.gguf"), CodecDirectory: codec}}}
	input := map[string]any{"modelCode": "local-fixture", "name": "Local fixture", "local": local}
	var provider localProvider
	requireNoError(t, process.Request(ctx, http.MethodPost, "/v1/providers/local/models", input, &provider))
	if provider.AuthMethod != "none" || !provider.Builtin || len(provider.Models) != 1 || provider.Models[0].Local.Resources[0].TokenizerSHA256 == "" {
		t.Fatalf("local provider contract: %+v", provider)
	}
	requireAPIError(t, process.Request(ctx, http.MethodPatch, "/v1/providers/local", map[string]any{"apiKey": "should-not-be-saved"}, nil), "invalid_params")
	requireAPIError(t, process.Request(ctx, http.MethodPost, "/v1/initialization", map[string]any{"providerCode": "local", "modelCode": "local-fixture"}, nil), "invalid_params")
	requireNoError(t, process.Close())
	process = startCoreAt(t, directory, coreEnv(directory))
	var restored localConfig
	requireNoError(t, process.Request(ctx, http.MethodGet, "/v1/local-inference/config", nil, &restored))
	if !restored.Enabled || len(restored.Workers) != 1 || restored.Workers[0].Command != command {
		t.Fatalf("lost worker config after restart: %+v", restored)
	}
	requireNoError(t, process.Request(ctx, http.MethodGet, "/v1/providers/local", nil, &provider))
	if len(provider.Models) != 1 || provider.Models[0].Local.OutputParser != "qwen3" {
		t.Fatalf("lost local resources: %+v", provider)
	}
	requireNoError(t, process.Request(ctx, http.MethodDelete, "/v1/providers/local/models/local-fixture", nil, nil))
	if _, err := os.Stat(filepath.Join(codec, "tokenizer.json")); err != nil {
		t.Fatal("model removal affected tokenizer files:", err)
	}
}

func TestLiveLocalRoundOverHTTP2(t *testing.T) {
	directory := os.Getenv("AGENTY_TEST_LOCAL_MODEL_DIR")
	if directory == "" {
		t.Skip("set local worker/model environment variables for real inference")
	}
	for _, backend := range []string{"mlx", "llamaCpp"} {
		t.Run(string(backend), func(t *testing.T) {
			command := os.Getenv("AGENTY_TEST_MLX_PYTHON")
			script, err := filepath.Abs("../../../mlx-worker/mlx_worker/__main__.py")
			requireNoError(t, err)
			if installedScript := os.Getenv("AGENTY_TEST_MLX_WORKER_SCRIPT"); installedScript != "" {
				script = installedScript
			}
			args := []string{script}
			resource := localResource{ID: "qwen35", Backend: backend, Format: "safetensors", WeightsPath: directory, CodecDirectory: directory}
			if backend == "llamaCpp" {
				command = os.Getenv("AGENTY_TEST_LLAMA_WORKER")
				args = nil
				resource.Format = "gguf"
				resource.WeightsPath = os.Getenv("AGENTY_TEST_LOCAL_GGUF")
			}
			if command == "" {
				t.Skip("worker not configured")
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
			defer cancel()
			process := startCore(t)
			requireNoError(t, process.Request(ctx, http.MethodPatch, "/v1/local-inference/config", localConfig{Workers: []localWorker{{ID: "live", Backend: backend, Enabled: true, Command: command, Arguments: args}}}, nil))
			local := &localModel{OutputParser: "qwen3_5", ThinkingEffort: "medium", Resources: []localResource{resource}}
			requireNoError(t, process.Request(ctx, http.MethodPost, "/v1/local-inference/model-probes", local, nil))
			requireNoError(t, process.Request(ctx, http.MethodPost, "/v1/providers/local/models", map[string]any{"modelCode": "qwen35", "name": "Qwen3.5 0.8B", "local": local, "contextWindow": 8192, "maxOutputTokens": 128}, nil))
			var beforeSave localConfig
			requireNoError(t, process.Request(ctx, http.MethodGet, "/v1/local-inference/config", nil, &beforeSave))
			if beforeSave.Enabled {
				t.Fatal("model preflight enabled local inference before saving setup")
			}
			requireNoError(t, process.Request(ctx, http.MethodPatch, "/v1/local-inference/config", map[string]any{"enabled": true}, nil))
			client := newAgentyClient(process)
			_, err = client.CompleteInitialization(ctx, "local", "qwen35")
			requireNoError(t, err)
			session, err := client.CreateSession(ctx, SessionCreateInput{ProviderCode: "local", ModelCode: "qwen35", ContextWindow: 8192})
			requireNoError(t, err)
			started, err := client.StartSession(ctx, session.ID, []ContentInput{{Type: "text", Text: "Say hello briefly. Do not use tools."}})
			requireNoError(t, err)
			completed, err := client.WaitForRoundStatus(ctx, session.ID, started.RoundID, "completed")
			requireNoError(t, err)
			if len(completed.Rounds) != 1 || len(completed.Rounds[0].Messages) < 2 {
				t.Fatalf("missing persisted model response: %+v", completed)
			}
			t.Logf("completed real %s round with %d messages", backend, len(completed.Rounds[0].Messages))
		})
	}
}
