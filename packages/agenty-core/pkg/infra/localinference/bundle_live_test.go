//go:build integration

package localinference

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	json "github.com/bytedance/sonic"
	domain "github.com/masteryyh/agenty-core/pkg/domain/localinference"
	"github.com/masteryyh/agenty-core/pkg/infra/llmcodec"
	"github.com/masteryyh/agenty-core/pkg/infra/workeripc"
)

func TestLiveWorkerBundleRelocation(t *testing.T) {
	directory := os.Getenv("AGENTY_TEST_WORKER_BUNDLE_DIR")
	if directory == "" {
		t.Skip("set AGENTY_TEST_WORKER_BUNDLE_DIR to validate actual private-runtime bundles")
	}
	entries, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Base(entry) == "index.json" {
			continue
		}
		t.Run(filepath.Base(entry), func(t *testing.T) {
			data, err := os.ReadFile(entry)
			if err != nil {
				t.Fatal(err)
			}
			var bundle Bundle
			if err := json.Unmarshal(data, &bundle); err != nil {
				t.Fatal(err)
			}
			archive := entry[:len(entry)-len(".json")] + ".zip"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, archive) }))
			defer server.Close()
			activated := make(chan domain.Worker, 1)
			updates := make(chan Installation, 4096)
			installer := NewInstaller(t.Context(), t.TempDir(), func(state Installation) { updates <- state }, func(worker domain.Worker) error { activated <- worker; return nil })
			if _, err := installer.Start(InstallInput{URL: server.URL, SHA256: bundle.SHA256}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			var worker domain.Worker
			for worker.Command == "" {
				select {
				case worker = <-activated:
				case state := <-updates:
					if state.Status == "failed" {
						t.Fatal(state.Error)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			client, err := workeripc.Start(ctx, worker.Command, worker.Arguments, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close(ctx)
			var info workeripc.Initialization
			if err := client.Call(ctx, "initialize", map[string]any{"protocolVersion": 1, "maxFrameBytes": workeripc.MaxFrameBytes}, &info, nil); err != nil {
				t.Fatal(err)
			}
			if modelDirectory := os.Getenv("AGENTY_TEST_LOCAL_MODEL_DIR"); modelDirectory != "" {
				resource := domain.Resource{ID: "relocated", Backend: worker.Backend, Format: "safetensors", WeightsPath: modelDirectory, CodecDirectory: modelDirectory}
				options := llmcodec.LoadOptions{IgnoreGGUF: true}
				if worker.Backend == domain.LlamaCpp {
					resource.Format = "gguf"
					resource.WeightsPath = os.Getenv("AGENTY_TEST_LOCAL_GGUF")
					options = llmcodec.LoadOptions{GGUFMetadataPath: resource.WeightsPath}
				}
				resource.TokenizerSHA256, err = TokenizerFingerprint(modelDirectory)
				if err != nil {
					t.Fatal(err)
				}
				codec, err := llmcodec.Load(modelDirectory, options)
				if err != nil {
					t.Fatal(err)
				}
				defer codec.Close()
				var loaded LoadedModel
				if err := client.Call(ctx, "loadModel", resource, &loaded, nil); err != nil {
					t.Fatal(err)
				}
				ids, err := codec.ChatEncode(map[string]any{"messages": []any{map[string]any{"role": "user", "content": "Reply OK."}}, "addGenerationPrompt": true, "templateKwargs": map[string]any{"enable_thinking": false}})
				if err != nil {
					t.Fatal(err)
				}
				var result struct {
					Usage struct {
						Output int `json:"outputTokens"`
					} `json:"usage"`
				}
				if err := client.Call(ctx, "generate", map[string]any{"modelHandle": loaded.ModelHandle, "tokenIds": ids, "maxOutputTokens": 32, "stopTokenIds": codec.Metadata.StopTokenIDs}, &result, nil); err != nil {
					t.Fatal(err)
				}
				if result.Usage.Output <= 0 {
					t.Fatal("private runtime did not generate tokens")
				}
			}
			if info.Backend != string(worker.Backend) {
				t.Fatalf("relocated worker reports wrong backend: %+v", info)
			}
		})
	}
}
