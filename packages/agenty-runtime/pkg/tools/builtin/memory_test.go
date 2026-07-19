package builtin

import (
	"context"
	"testing"

	"github.com/masteryyh/agenty/pkg/tools"
)

func TestSaveMemoryToolDefinition(t *testing.T) {
	tool := &SaveMemoryTool{}
	def := tool.Definition()

	if def.Name != "save_memory" {
		t.Fatalf("expected name 'save_memory', got '%s'", def.Name)
	}
	if _, ok := def.Parameters.Properties["content"]; !ok {
		t.Fatal("expected 'content' parameter in definition")
	}
	if len(def.Parameters.Required) != 1 || def.Parameters.Required[0] != "content" {
		t.Fatalf("expected required=['content'], got %v", def.Parameters.Required)
	}
}

func TestSaveMemoryToolEmptyContent(t *testing.T) {
	tool := &SaveMemoryTool{}
	_, err := tool.Execute(context.Background(), tools.ToolCallContext{}, `{"content": ""}`)
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestSaveMemoryToolInvalidJSON(t *testing.T) {
	tool := &SaveMemoryTool{}
	_, err := tool.Execute(context.Background(), tools.ToolCallContext{}, `invalid json`)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
