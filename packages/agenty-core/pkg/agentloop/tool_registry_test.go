package agentloop_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

type testTool struct {
	definition agentloop.ToolDefinition
	execute    func(context.Context, agentloop.CallContext, []byte) (conversation.Content, error)
}

func (tool *testTool) Definition() agentloop.ToolDefinition {
	return tool.definition
}

func (tool *testTool) Execute(
	ctx context.Context,
	callContext agentloop.CallContext,
	input []byte,
) (conversation.Content, error) {
	return tool.execute(ctx, callContext, input)
}

func TestRegistryRegistrationAndDefinitions(t *testing.T) {
	t.Parallel()

	registry := agentloop.NewRegistry()
	for _, name := range []string{"zeta", "alpha"} {
		tool := &testTool{
			definition: agentloop.ToolDefinition{Name: name},
			execute: func(context.Context, agentloop.CallContext, []byte) (conversation.Content, error) {
				return conversation.Text("ok"), nil
			},
		}
		if err := registry.Register(tool); err != nil {
			t.Fatal(err)
		}
	}

	definitions := registry.Definitions()
	if len(definitions) != 2 || definitions[0].Name != "alpha" || definitions[1].Name != "zeta" {
		t.Fatalf("definitions = %+v, want alpha then zeta", definitions)
	}
	if err := registry.Register(&testTool{definition: agentloop.ToolDefinition{Name: "alpha"}}); err == nil {
		t.Error("Register accepted a duplicate tool")
	}
	if err := registry.Register(nil); err == nil {
		t.Error("Register accepted nil")
	}
	if err := registry.Register(&testTool{}); err == nil {
		t.Error("Register accepted an empty name")
	}

	registry.Unregister("alpha")
	if _, ok := registry.Get("alpha"); ok {
		t.Error("Unregister left alpha in the registry")
	}
}

func TestRegistryExecuteBatchRunsInParallelAndPreservesOrder(t *testing.T) {
	t.Parallel()

	registry := agentloop.NewRegistry()
	release := make(chan struct{})
	started := make(chan struct{}, 3)
	var active atomic.Int32
	var peak atomic.Int32

	for _, name := range []string{"first", "second", "third"} {
		toolName := name
		tool := &testTool{
			definition: agentloop.ToolDefinition{Name: toolName},
			execute: func(ctx context.Context, _ agentloop.CallContext, _ []byte) (conversation.Content, error) {
				current := active.Add(1)
				defer active.Add(-1)
				for {
					observed := peak.Load()
					if current <= observed || peak.CompareAndSwap(observed, current) {
						break
					}
				}
				started <- struct{}{}

				select {
				case <-release:
					return conversation.Text(toolName), nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			},
		}
		if err := registry.Register(tool); err != nil {
			t.Fatal(err)
		}
	}

	resultChannel := make(chan []conversation.ToolResultBlock, 1)
	go func() {
		resultChannel <- registry.ExecuteBatch(t.Context(), agentloop.CallContext{}, []conversation.ToolUseBlock{
			{ID: "1", Name: "first", Input: []byte(`{}`)},
			{ID: "2", Name: "second", Input: []byte(`{}`)},
			{ID: "3", Name: "third", Input: []byte(`{}`)},
		})
	}()

	for range 3 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("tools did not start concurrently")
		}
	}
	close(release)
	results := <-resultChannel

	if peak.Load() != 3 {
		t.Errorf("peak active tools = %d, want 3", peak.Load())
	}
	for index, want := range []string{"first", "second", "third"} {
		if results[index].ToolUseID != string(rune('1'+index)) {
			t.Errorf("result %d tool use id = %q", index, results[index].ToolUseID)
		}
		block, ok := results[index].Content[0].(conversation.TextBlock)
		if !ok || block.Text != want {
			t.Errorf("result %d content = %+v, want %q", index, results[index].Content, want)
		}
	}
}

func TestRegistryExecuteBatchIsolatesFailures(t *testing.T) {
	t.Parallel()

	registry := agentloop.NewRegistry()
	if err := registry.Register(&testTool{
		definition: agentloop.ToolDefinition{Name: "failing"},
		execute: func(context.Context, agentloop.CallContext, []byte) (conversation.Content, error) {
			return nil, errors.New("test failure")
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(&testTool{
		definition: agentloop.ToolDefinition{Name: "panicking"},
		execute: func(context.Context, agentloop.CallContext, []byte) (conversation.Content, error) {
			panic("test panic")
		},
	}); err != nil {
		t.Fatal(err)
	}

	results := registry.ExecuteBatch(t.Context(), agentloop.CallContext{}, []conversation.ToolUseBlock{
		{ID: "missing", Name: "missing"},
		{ID: "failing", Name: "failing"},
		{ID: "panicking", Name: "panicking"},
	})
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	for _, result := range results {
		if !result.IsError || len(result.Content) != 1 {
			t.Errorf("failure result = %+v", result)
		}
	}
}
