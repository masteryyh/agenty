//go:build e2e

package e2e_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestModelHubHTTP2StorageAndRecoverySnapshot(t *testing.T) {
	t.Parallel()
	ctx, cancel := testContext(t)
	defer cancel()
	process := startCore(t)
	var storage struct {
		Directory string `json:"directory"`
		Volume    string `json:"volume"`
		Total     uint64 `json:"totalBytes"`
		Available uint64 `json:"availableBytes"`
	}
	requireNoError(t, process.Request(ctx, http.MethodGet, "/v1/local-inference/model-storage", nil, &storage))
	if storage.Total == 0 || storage.Volume == "" || filepath.Base(storage.Directory) != "models" {
		t.Fatalf("invalid storage: %+v", storage)
	}
	if _, err := os.Stat(storage.Directory); !os.IsNotExist(err) {
		t.Fatalf("storage preview created a directory: %v", err)
	}
	var downloads []json.RawMessage
	requireNoError(t, process.Request(ctx, http.MethodGet, "/v1/local-inference/model-downloads", nil, &downloads))
	if len(downloads) != 0 {
		t.Fatal("new core has model downloads")
	}
	requireAPIError(t, process.Request(ctx, http.MethodPost, "/v1/local-inference/model-downloads", map[string]any{"planId": "expired"}, nil), "invalid_params")
	requireAPIError(t, process.Request(ctx, http.MethodGet, "/v1/local-inference/hub-models?source=unknown", nil, nil), "invalid_params")

	reader, writer := io.Pipe()
	defer writer.Close()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://agenty.local/v1/stream", reader)
	requireNoError(t, err)
	request.Header.Set("Content-Type", "application/x-ndjson")
	response, err := process.client.Do(request)
	requireNoError(t, err)
	defer response.Body.Close()
	_, err = writer.Write([]byte("{\"type\":\"subscribe\",\"topic\":\"localInference\"}\n"))
	requireNoError(t, err)
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		var frame struct {
			Type     string `json:"type"`
			Snapshot struct {
				State map[string]json.RawMessage `json:"state"`
			} `json:"snapshot"`
		}
		requireNoError(t, json.Unmarshal(scanner.Bytes(), &frame))
		if frame.Type == "snapshot" {
			if string(frame.Snapshot.State["modelDownloads"]) != "[]" {
				t.Fatalf("snapshot lacks download recovery: %s", scanner.Text())
			}
			return
		}
	}
	t.Fatalf("no local inference snapshot: %v", scanner.Err())
}

func TestLiveDownloadedModelRound(t *testing.T) {
	source := os.Getenv("AGENTY_TEST_MODEL_HUB_DOWNLOAD")
	command := os.Getenv("AGENTY_TEST_LLAMA_WORKER")
	if source == "" || command == "" {
		t.Skip("set AGENTY_TEST_MODEL_HUB_DOWNLOAD and AGENTY_TEST_LLAMA_WORKER for a real weight download and inference")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Minute)
	defer cancel()
	process := startCore(t)
	repository := os.Getenv("AGENTY_TEST_DOWNLOAD_REPOSITORY")
	variant := os.Getenv("AGENTY_TEST_DOWNLOAD_VARIANT")
	codecRepository := os.Getenv("AGENTY_TEST_DOWNLOAD_TOKENIZER")
	if repository == "" {
		repository, variant, codecRepository = "bartowski/Qwen_Qwen3-0.6B-GGUF", "Qwen_Qwen3-0.6B-Q4_K_M.gguf", "Qwen/Qwen3-0.6B"
	}
	requireNoError(t, process.Request(ctx, http.MethodPatch, "/v1/local-inference/config", localConfig{Workers: []localWorker{{ID: "download-test", Backend: "llamaCpp", Enabled: true, Command: command}}}, nil))
	var plan struct {
		ID        string `json:"id"`
		Supported bool   `json:"supported"`
		Reason    string `json:"reason"`
		Enough    bool   `json:"enoughSpace"`
		Total     int64  `json:"totalBytes"`
	}
	requireNoError(t, process.Request(ctx, http.MethodPost, "/v1/local-inference/model-download-plans", map[string]any{"source": source, "repository": repository, "variant": variant, "codecRepository": codecRepository}, &plan))
	if !plan.Supported || !plan.Enough {
		t.Fatalf("model not ready to download: %+v", plan)
	}
	var started struct {
		ID string `json:"id"`
	}
	requireNoError(t, process.Request(ctx, http.MethodPost, "/v1/local-inference/model-downloads", map[string]any{"planId": plan.ID}, &started))
	t.Logf("downloading %d bytes from %s", plan.Total, source)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	code := ""
	for code == "" {
		var states []struct {
			ID        string `json:"id"`
			Status    string `json:"status"`
			ModelCode string `json:"modelCode"`
			Error     string `json:"error"`
		}
		requireNoError(t, process.Request(ctx, http.MethodGet, "/v1/local-inference/model-downloads", nil, &states))
		for _, state := range states {
			if state.ID != started.ID {
				continue
			}
			switch state.Status {
			case "ready":
				code = state.ModelCode
			case "downloaded", "failed", "cancelled":
				t.Fatalf("download failed: %+v", state)
			}
		}
		if code != "" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	requireNoError(t, process.Request(ctx, http.MethodPatch, "/v1/local-inference/config", map[string]any{"enabled": true}, nil))
	client := newAgentyClient(process)
	_, err := client.CompleteInitialization(ctx, "local", code)
	requireNoError(t, err)
	session, err := client.CreateSession(ctx, SessionCreateInput{ProviderCode: "local", ModelCode: code, ContextWindow: 8192})
	requireNoError(t, err)
	round, err := client.StartSession(ctx, session.ID, []ContentInput{{Type: "text", Text: "Say hello briefly. Do not use tools."}})
	requireNoError(t, err)
	completed, err := client.WaitForRoundStatus(ctx, session.ID, round.RoundID, "completed")
	requireNoError(t, err)
	if len(completed.Rounds) == 0 || len(completed.Rounds[0].Messages) < 2 {
		t.Fatal("downloaded model did not persist an answer")
	}
	t.Logf("downloaded model %s completed an HTTP/2 inference round", code)
}
