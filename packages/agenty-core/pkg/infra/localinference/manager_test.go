package localinference

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	domain "github.com/masteryyh/agenty-core/pkg/domain/localinference"
	"github.com/masteryyh/agenty-core/pkg/infra/workeripc"
)

func TestManagerWorkerFixture(t *testing.T) {
	if !slices.Contains(os.Args, "agenty-manager-fixture") {
		return
	}
	input := bufio.NewScanner(os.Stdin)
	output := json.NewEncoder(os.Stdout)
	for input.Scan() {
		var request workeripc.Frame
		if json.Unmarshal(input.Bytes(), &request) != nil {
			os.Exit(2)
		}
		var result any = struct{}{}
		switch request.Method {
		case "initialize":
			result = workeripc.Initialization{ProtocolVersion: 1, Backend: "llamaCpp", EngineVersion: strconv.Itoa(os.Getpid()), MaxFrameBytes: workeripc.MaxFrameBytes, MaxConcurrentGenerations: 1, MaxInputTokens: 8192, MaxOutputTokens: 8192}
		case "probeModel":
			result = map[string]any{"compatible": true}
		case "loadModel":
			params := request.Params.(map[string]any)
			result = LoadedModel{ModelHandle: "fixture-model", ContextWindow: 8192, VocabSize: 1000000, TokenizerSHA256: params["tokenizerSha256"].(string)}
		case "crash":
			os.Exit(9)
		}
		data, _ := json.Marshal(result)
		_ = output.Encode(workeripc.Frame{Version: 1, Type: "response", RequestID: request.RequestID, Result: data})
		if request.Method == "shutdown" {
			os.Exit(0)
		}
	}
	os.Exit(0)
}

func TestLeaseIsolationAndAcceptedCallIsNotReplayed(t *testing.T) {
	command, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	codec, err := filepath.Abs("../../../../llm-codec/tests/fixtures/basic")
	if err != nil {
		t.Fatal(err)
	}
	weights := filepath.Join(t.TempDir(), "metadata.gguf")
	metadata := make([]byte, 24)
	copy(metadata, "GGUF")
	binary.LittleEndian.PutUint32(metadata[4:], 3)
	if err := os.WriteFile(weights, metadata, 0600); err != nil {
		t.Fatal(err)
	}
	m := New(t.Context(), nil)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()
	if err := m.Configure(domain.Config{Enabled: true, Workers: []domain.Worker{{ID: "fixture", Backend: domain.LlamaCpp, Enabled: true, Command: command, Arguments: []string{"-test.run=^TestManagerWorkerFixture$", "--", "agenty-manager-fixture"}}}}); err != nil {
		t.Fatal(err)
	}
	model := &domain.Model{OutputParser: "qwen3", Resources: []domain.Resource{
		{ID: "first", Backend: domain.LlamaCpp, Format: "gguf", WeightsPath: weights, CodecDirectory: codec},
		{ID: "second", Backend: domain.LlamaCpp, Format: "gguf", WeightsPath: weights, CodecDirectory: codec},
	}}
	acceptedFailure := errors.New("failure after generation accepted")
	calls := 0
	if err := m.WithModel(t.Context(), model, func(Lease) error { calls++; return acceptedFailure }); !errors.Is(err, acceptedFailure) {
		t.Fatal(err)
	}
	if calls != 1 || m.Snapshot().Workers[0].ModelResourceID != "first" {
		t.Fatal("accepted call replayed on an alternate resource")
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- m.WithModel(t.Context(), model, func(lease Lease) error { close(entered); <-release; return lease.Context.Err() })
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first lease did not start")
	}
	queued, cancel := context.WithCancel(t.Context())
	cancel()
	if err := m.WithModel(queued, model, func(Lease) error { t.Error("cancelled queued call was executed"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal("queued cancellation:", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal("another caller's cancellation affected active lease:", err)
	}
	if _, err := m.Probe(t.Context(), &domain.Model{OutputParser: "qwen3", Resources: model.Resources[1:]}); err != nil {
		t.Fatal(err)
	}
	if m.Snapshot().Workers[0].ModelResourceID != "first" {
		t.Fatal("preflight evicted a loaded model")
	}
}

func TestDisabledInferenceProbeUsesTemporaryWorker(t *testing.T) {
	command, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	codec, err := filepath.Abs("../../../../llm-codec/tests/fixtures/basic")
	if err != nil {
		t.Fatal(err)
	}
	weights := filepath.Join(t.TempDir(), "metadata.gguf")
	metadata := make([]byte, 24)
	copy(metadata, "GGUF")
	binary.LittleEndian.PutUint32(metadata[4:], 3)
	if err := os.WriteFile(weights, metadata, 0600); err != nil {
		t.Fatal(err)
	}
	m := New(t.Context(), nil)
	if err := m.Configure(domain.Config{Workers: []domain.Worker{{ID: "fixture", Backend: domain.LlamaCpp, Enabled: true, Command: command, Arguments: []string{"-test.run=^TestManagerWorkerFixture$", "--", "agenty-manager-fixture"}}}}); err != nil {
		t.Fatal(err)
	}
	model := &domain.Model{OutputParser: "qwen3", Resources: []domain.Resource{{ID: "gguf", Backend: domain.LlamaCpp, Format: "gguf", WeightsPath: weights, CodecDirectory: codec}}}
	resource, err := m.Probe(t.Context(), model)
	if err != nil || resource.ID != "gguf" {
		t.Fatal(resource, err)
	}
	state := m.Snapshot()
	if state.Enabled || len(state.Workers) != 1 || state.Workers[0].Status != "stopped" || state.Workers[0].EngineVersion != "" {
		t.Fatalf("preflight enabled background inference: %+v", state)
	}
	if err := m.WithModel(t.Context(), model, func(Lease) error { t.Error("disabled inference generated"); return nil }); err == nil {
		t.Fatal("preflight enabled inference")
	}
	if err := m.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func waitWorkerState(t *testing.T, m *Manager, predicate func(domain.Snapshot) bool) domain.Snapshot {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		state := m.Snapshot()
		if predicate(state) {
			return state
		}
		select {
		case <-timer.C:
			t.Fatalf("worker state did not converge: %+v", state)
		case <-tick.C:
		}
	}
}

func TestManagerDisabledReuseReplacementCrashAndShutdown(t *testing.T) {
	command, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	worker := domain.Worker{ID: "native", Backend: domain.LlamaCpp, Enabled: true, Command: command, Arguments: []string{"-test.run=^TestManagerWorkerFixture$", "--", "agenty-manager-fixture"}}
	m := New(t.Context(), nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	cfg := domain.Config{Workers: []domain.Worker{worker}}
	if err := m.Configure(cfg); err != nil {
		t.Fatal(err)
	}
	if state := m.Snapshot(); state.Enabled || state.Workers[0].Status != "stopped" || state.Workers[0].EngineVersion != "" {
		t.Fatalf("disabled configuration started worker: %+v", state)
	}
	cfg.Enabled = true
	if err := m.Configure(cfg); err != nil {
		t.Fatal(err)
	}
	first := waitWorkerState(t, m, func(s domain.Snapshot) bool { return s.Workers[0].Status == "ready" })
	if err := m.Configure(cfg); err != nil {
		t.Fatal(err)
	}
	if got := m.Snapshot().Workers[0].EngineVersion; got != first.Workers[0].EngineVersion {
		t.Fatal("unchanged config replaced a live worker")
	}
	m.mu.Lock()
	old := m.slots[worker.ID]
	m.mu.Unlock()
	old.gate <- struct{}{}
	cfg.Workers[0].Version = "replacement"
	if err := m.Configure(cfg); err != nil {
		t.Fatal(err)
	}
	if !old.client.Alive() {
		t.Fatal("replacement stopped the active old worker")
	}
	if got := m.Snapshot().Workers[0].EngineVersion; got != "" {
		t.Fatal("replacement started before old call released")
	}
	previousClient := old.client
	<-old.gate
	second := waitWorkerState(t, m, func(s domain.Snapshot) bool { return s.Workers[0].Status == "ready" })
	if second.Workers[0].EngineVersion == first.Workers[0].EngineVersion {
		t.Fatal("worker version was not replaced")
	}
	select {
	case <-previousClient.Exited():
	default:
		t.Fatal("old worker was not reaped before replacement")
	}
	m.mu.Lock()
	current := m.slots[worker.ID]
	m.mu.Unlock()
	current.gate <- struct{}{}
	if err := current.client.Call(t.Context(), "crash", struct{}{}, nil, nil); err == nil {
		t.Error("worker crash reported success")
	}
	<-current.gate
	waitWorkerState(t, m, func(s domain.Snapshot) bool { return s.Workers[0].Status == "failed" && s.Workers[0].Error != "" })
	cfg.Enabled = false
	if err := m.Configure(cfg); err != nil {
		t.Fatal(err)
	}
	if state := m.Snapshot(); state.Enabled || state.Workers[0].Status != "stopped" {
		t.Fatalf("disable did not stop the worker: %+v", state)
	}
}

type memoryConfigStore struct {
	value     domain.Config
	saveError error
}

func (s *memoryConfigStore) LocalInference() domain.Config { return s.value }
func (s *memoryConfigStore) SetLocalInference(c domain.Config) error {
	if s.saveError != nil {
		return s.saveError
	}
	s.value = c
	return nil
}

func TestConfigPersistenceFailureDoesNotActivateOrPublish(t *testing.T) {
	published := 0
	m := New(t.Context(), func(domain.Snapshot) { published++ })
	failure := errors.New("disk write failed")
	store := &memoryConfigStore{saveError: failure}
	_, err := m.UpdateConfig(store, func(c *domain.Config) { c.Enabled = true })
	if !errors.Is(err, failure) || m.Snapshot().Enabled || store.value.Enabled || published != 0 {
		t.Fatalf("unsaved configuration became visible: %v %+v publications=%d", err, m.Snapshot(), published)
	}
}

func TestConcurrentConfigUpdatesPreserveAllChanges(t *testing.T) {
	command, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m := New(t.Context(), nil)
	store := &memoryConfigStore{value: domain.Config{Workers: []domain.Worker{{ID: "native", Backend: domain.LlamaCpp, Command: command, Version: "0"}}}}
	var wait sync.WaitGroup
	for range 30 {
		wait.Go(func() {
			_, err := m.UpdateConfig(store, func(c *domain.Config) {
				version, _ := strconv.Atoi(c.Workers[0].Version)
				c.Workers[0].Version = strconv.Itoa(version + 1)
			})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wait.Wait()
	if store.value.Workers[0].Version != "30" {
		t.Fatalf("lost concurrent update: %+v", store.value)
	}
	if err := m.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}
