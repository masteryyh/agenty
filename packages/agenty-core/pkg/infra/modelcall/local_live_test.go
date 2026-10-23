//go:build integration

package modelcall

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	domain "github.com/masteryyh/agenty-core/pkg/domain/localinference"
	localruntime "github.com/masteryyh/agenty-core/pkg/infra/localinference"
)

func TestLiveLocalQwen35(t *testing.T) {
	directory := os.Getenv("AGENTY_TEST_LOCAL_MODEL_DIR")
	if directory == "" {
		t.Skip("set AGENTY_TEST_LOCAL_MODEL_DIR and a worker executable for real model validation")
	}
	for _, backend := range []domain.Backend{domain.MLX, domain.LlamaCpp} {
		t.Run(string(backend), func(t *testing.T) {
			executable := os.Getenv("AGENTY_TEST_MLX_PYTHON")
			script, err := filepath.Abs("../../../../mlx-worker/mlx_worker/__main__.py")
			if err != nil {
				t.Fatal(err)
			}
			args := []string{script}
			resource := domain.Resource{ID: "qwen35", Backend: backend, Format: "safetensors", WeightsPath: directory, CodecDirectory: directory}
			if backend == domain.LlamaCpp {
				executable = os.Getenv("AGENTY_TEST_LLAMA_WORKER")
				args = nil
				resource.Format = "gguf"
				resource.WeightsPath = os.Getenv("AGENTY_TEST_LOCAL_GGUF")
			}
			if executable == "" {
				t.Skip("worker executable not configured")
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
			defer cancel()
			manager := localruntime.New(ctx, nil)
			if err := manager.Configure(domain.Config{Enabled: true, Workers: []domain.Worker{{ID: "test", Backend: backend, Enabled: true, Command: executable, Arguments: args}}}); err != nil {
				t.Fatal(err)
			}
			defer func() {
				shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
				defer stop()
				_ = manager.Shutdown(shutdown)
			}()
			client := &Client{LocalRuntime: manager}
			model := ModelCallConfig{APIType: APILocal, ModelCode: "qwen3.5-0.8b", ContextWindow: 8192, Local: &domain.Model{OutputParser: "qwen3_5", ThinkingEffort: "medium", Resources: []domain.Resource{resource}}}
			request := ModelCallRequest{SystemPrompt: "Answer briefly.", Messages: []ModelCallMessage{{Role: conversation.RoleUser, Content: conversation.Text("用中文说一句你好。")}}, MaxOutputTokens: 64}
			var streamed strings.Builder
			response, err := client.Invoke(ctx, model, request, func(event ModelCallStreamEvent) error {
				if event.Type == ModelCallStreamEventTextDelta {
					streamed.WriteString(event.Delta)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			text, err := textContent(response.Content)
			if err != nil || text == "" || text != streamed.String() {
				t.Fatalf("stream did not converge: %+v text=%q err=%v", response, text, err)
			}
			t.Logf("text: %q; tokens %+v", text, response.Usage)
			request.Messages[0].Content = conversation.Text("What is 1+1? Answer briefly.")
			request.ReasoningEffort = "medium"
			response, err = client.Invoke(ctx, model, request, nil)
			if err != nil {
				t.Fatal(err)
			}
			foundReasoning := false
			for _, block := range response.Content {
				if b, ok := block.(conversation.ReasoningBlock); ok && b.Reasoning != "" {
					foundReasoning = true
				}
			}
			if !foundReasoning {
				t.Fatalf("missing reasoning: %+v", response)
			}
			request.ReasoningEffort = "off"
			request.SystemPrompt = "You are a tool calling assistant. Always use the available tool to obtain the answer. Never answer from memory."
			request.Messages[0].Content = conversation.Text("What is the current weather in Beijing? Call get_weather to look it up.")
			request.Tools = []ToolDefinition{{Name: "get_weather", Description: "Look up the current weather for a city.", InputSchema: JSONSchema{Type: JSONSchemaTypeObject, Properties: map[string]JSONSchema{"city": {Type: JSONSchemaTypeString}}, Required: []string{"city"}}}}
			request.MaxOutputTokens = 256
			response, err = client.Invoke(ctx, model, request, nil)
			if err != nil {
				t.Fatal(err)
			}
			if response.StopReason != ModelCallStopReasonToolUse {
				t.Fatalf("model did not call tool: %+v", response)
			}
			t.Logf("tool response: %+v", response.Content)
			request.Messages = append(request.Messages, ModelCallMessage{Role: conversation.RoleAssistant, Content: response.Content})
			for _, block := range response.Content {
				if call, ok := block.(conversation.ToolUseBlock); ok {
					request.Messages = append(request.Messages, ModelCallMessage{Role: conversation.RoleUser, Content: conversation.Content{conversation.ToolResultBlock{ToolUseID: call.ID, Content: conversation.Text("Sunny, 20 degrees Celsius.")}}})
				}
			}
			request.Tools = nil
			response, err = client.Invoke(ctx, model, request, nil)
			if err != nil {
				t.Fatal(err)
			}
			if response.StopReason == ModelCallStopReasonToolUse {
				t.Fatal("tool continuation did not complete")
			}
			cancelCtx, cancelGeneration := context.WithCancel(ctx)
			request.Messages = []ModelCallMessage{{Role: conversation.RoleUser, Content: conversation.Text("Count from 1 to 1000.")}}
			_, err = client.Invoke(cancelCtx, model, request, func(event ModelCallStreamEvent) error {
				if event.Type == ModelCallStreamEventTextDelta {
					cancelGeneration()
				}
				return nil
			})
			cancelGeneration()
			if err != context.Canceled {
				t.Fatalf("cancellation result: %v", err)
			}
			request.Messages[0].Content = conversation.Text("Reply OK.")
			if _, err := client.Invoke(ctx, model, request, nil); err != nil {
				t.Fatalf("worker unusable after cancellation: %v", err)
			}
			// Cancel while the engine is prefilling a long raw-token prompt,
			// before waiting for the first generated text delta.
			err = manager.WithModel(ctx, model.Local, func(lease localruntime.Lease) error {
				ids, err := lease.Codec.Tokenize(strings.Repeat("test ", 5000))
				if err != nil {
					return err
				}
				prefillCtx, stop := context.WithCancel(ctx)
				defer stop()
				timer := time.AfterFunc(50*time.Millisecond, stop)
				defer timer.Stop()
				return lease.Client.Call(prefillCtx, "generate", map[string]any{"modelHandle": lease.Model.ModelHandle, "tokenIds": ids,
					"maxOutputTokens": 32, "stopTokenIds": lease.Codec.Metadata.StopTokenIDs}, nil, nil)
			})
			if err != context.Canceled {
				t.Fatalf("long prefill cancellation: %v", err)
			}
			if _, err := client.Invoke(ctx, model, request, nil); err != nil {
				t.Fatalf("worker unusable after prefill cancellation: %v", err)
			}
		})
	}
}

func TestLiveLocalFallbackBeforeGeneration(t *testing.T) {
	python, native, directory, gguf := os.Getenv("AGENTY_TEST_MLX_PYTHON"), os.Getenv("AGENTY_TEST_LLAMA_WORKER"), os.Getenv("AGENTY_TEST_LOCAL_MODEL_DIR"), os.Getenv("AGENTY_TEST_LOCAL_GGUF")
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" || python == "" || native == "" || directory == "" || gguf == "" {
		t.Skip("requires both real workers and Qwen3.5 resources on Apple Silicon")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	manager := localruntime.New(ctx, nil)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = manager.Shutdown(ctx)
	}()
	if err := manager.Configure(domain.Config{Enabled: true, Workers: []domain.Worker{
		{ID: "mlx", Backend: domain.MLX, Enabled: true, Command: python, Arguments: []string{"-c", "raise SystemExit(9)"}},
		{ID: "native", Backend: domain.LlamaCpp, Enabled: true, Command: native},
	}}); err != nil {
		t.Fatal(err)
	}
	client := &Client{LocalRuntime: manager}
	model := ModelCallConfig{APIType: APILocal, ModelCode: "qwen35", ContextWindow: 8192, Local: &domain.Model{OutputParser: "qwen3_5", Resources: []domain.Resource{
		{ID: "mlx", Backend: domain.MLX, Format: "safetensors", WeightsPath: directory, CodecDirectory: directory},
		{ID: "gguf", Backend: domain.LlamaCpp, Format: "gguf", WeightsPath: gguf, CodecDirectory: directory},
	}}}
	response, err := client.Invoke(ctx, model, ModelCallRequest{Messages: []ModelCallMessage{{Role: conversation.RoleUser, Content: conversation.Text("Reply OK.")}}, MaxOutputTokens: 32}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Content) == 0 {
		t.Fatal("fallback produced no content")
	}
	state := manager.Snapshot()
	if state.Workers[0].ID != "mlx" || state.Workers[0].Status != "failed" || state.Workers[0].Error == "" || state.Workers[1].ModelResourceID != "gguf" {
		t.Fatalf("fallback state: %+v", state)
	}
}
