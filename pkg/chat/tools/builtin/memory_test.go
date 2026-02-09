package builtin

import (
	"testing"
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

func TestSearchMemoryToolDefinition(t *testing.T) {
	tool := &SearchMemoryTool{}
	def := tool.Definition()

	if def.Name != "search_memory" {
		t.Fatalf("expected name 'search_memory', got '%s'", def.Name)
	}
	if _, ok := def.Parameters.Properties["query"]; !ok {
		t.Fatal("expected 'query' parameter in definition")
	}
	if len(def.Parameters.Required) != 1 || def.Parameters.Required[0] != "query" {
		t.Fatalf("expected required=['query'], got %v", def.Parameters.Required)
	}
}

func TestSaveMemoryToolEmptyContent(t *testing.T) {
	tool := &SaveMemoryTool{}
	_, err := tool.Execute(nil, `{"content": ""}`)
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestSaveMemoryToolInvalidJSON(t *testing.T) {
	tool := &SaveMemoryTool{}
	_, err := tool.Execute(nil, `invalid json`)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSearchMemoryToolEmptyQuery(t *testing.T) {
	tool := &SearchMemoryTool{}
	_, err := tool.Execute(nil, `{"query": ""}`)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestSearchMemoryToolInvalidJSON(t *testing.T) {
	tool := &SearchMemoryTool{}
	_, err := tool.Execute(nil, `invalid json`)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
