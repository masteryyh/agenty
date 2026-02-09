package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

type mockTool struct {
	name   string
	result string
	err    error
}

func (t *mockTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        t.name,
		Description: "mock tool for testing",
		Parameters: ToolParameters{
			Type: "object",
			Properties: map[string]ParameterProperty{
				"input": {Type: "string", Description: "test input"},
			},
			Required: []string{"input"},
		},
	}
}

func (t *mockTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return t.result, t.err
}

func newTestRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := newTestRegistry()
	tool := &mockTool{name: "test_tool", result: "ok"}

	r.Register(tool)

	got, ok := r.Get("test_tool")
	if !ok {
		t.Fatal("expected tool to be found")
	}
	if got.Definition().Name != "test_tool" {
		t.Fatalf("expected name 'test_tool', got '%s'", got.Definition().Name)
	}
}

func TestRegistryGetNotFound(t *testing.T) {
	r := newTestRegistry()

	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("expected tool not to be found")
	}
}

func TestRegistryAll(t *testing.T) {
	r := newTestRegistry()
	r.Register(&mockTool{name: "a", result: "ok"})
	r.Register(&mockTool{name: "b", result: "ok"})

	all := r.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(all))
	}
}

func TestRegistryDefinitions(t *testing.T) {
	r := newTestRegistry()
	r.Register(&mockTool{name: "tool1"})
	r.Register(&mockTool{name: "tool2"})

	defs := r.Definitions()
	if len(defs) != 2 {
		t.Fatalf("expected 2 definitions, got %d", len(defs))
	}

	names := make(map[string]bool)
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names["tool1"] || !names["tool2"] {
		t.Fatal("expected both tool1 and tool2 in definitions")
	}
}

func TestRegistryExecuteSuccess(t *testing.T) {
	r := newTestRegistry()
	r.Register(&mockTool{name: "echo", result: "hello world"})

	result := r.Execute(context.Background(), ToolCall{
		ID:        "call_1",
		Name:      "echo",
		Arguments: json.RawMessage(`{"input":"test"}`),
	})

	if result.IsError {
		t.Fatalf("expected no error, got: %s", result.Content)
	}
	if result.Content != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", result.Content)
	}
	if result.CallID != "call_1" {
		t.Fatalf("expected call_id 'call_1', got '%s'", result.CallID)
	}
}

func TestRegistryExecuteToolNotFound(t *testing.T) {
	r := newTestRegistry()

	result := r.Execute(context.Background(), ToolCall{
		ID:   "call_1",
		Name: "nonexistent",
	})

	if !result.IsError {
		t.Fatal("expected error for nonexistent tool")
	}
	if result.Content != "tool not found: nonexistent" {
		t.Fatalf("unexpected error message: %s", result.Content)
	}
}

func TestRegistryExecuteToolError(t *testing.T) {
	r := newTestRegistry()
	r.Register(&mockTool{name: "failing", err: fmt.Errorf("tool execution failed")})

	result := r.Execute(context.Background(), ToolCall{
		ID:        "call_1",
		Name:      "failing",
		Arguments: json.RawMessage(`{}`),
	})

	if !result.IsError {
		t.Fatal("expected error")
	}
}
