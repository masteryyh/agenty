/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package builtin

import (
	"context"
	"testing"

	"github.com/masteryyh/agenty/pkg/chat/tools"
)

func TestSearchToolDefinition(t *testing.T) {
	tool := &SearchTool{}
	def := tool.Definition()

	if def.Name != "search" {
		t.Fatalf("expected name 'search', got '%s'", def.Name)
	}
	searchesProp, ok := def.Parameters.Properties["searches"]
	if !ok {
		t.Fatal("expected 'searches' parameter in definition")
	}
	if searchesProp.Type != "array" {
		t.Fatalf("expected 'searches' to be array type, got '%s'", searchesProp.Type)
	}
	if searchesProp.Items == nil {
		t.Fatal("expected 'searches' items schema to be defined")
	}
	if len(def.Parameters.Required) != 1 || def.Parameters.Required[0] != "searches" {
		t.Fatalf("expected required=['searches'], got %v", def.Parameters.Required)
	}
}

func TestSearchToolEmptySearches(t *testing.T) {
	tool := &SearchTool{}
	_, err := tool.Execute(context.Background(), tools.ToolCallContext{}, `{"searches": []}`)
	if err == nil {
		t.Fatal("expected error for empty searches array")
	}
}

func TestSearchToolEmptyQuery(t *testing.T) {
	tool := &SearchTool{}
	_, err := tool.Execute(context.Background(), tools.ToolCallContext{}, `{"searches": [{"id": "1", "channel": "knowledge_base", "query": ""}]}`)
	if err == nil {
		t.Fatal("expected error for empty query in search spec")
	}
}

func TestSearchToolUnknownChannel(t *testing.T) {
	tool := &SearchTool{}
	_, err := tool.Execute(context.Background(), tools.ToolCallContext{}, `{"searches": [{"id": "1", "channel": "unknown", "query": "test"}]}`)
	if err == nil {
		t.Fatal("expected error for unknown channel")
	}
}

func TestSearchToolInvalidJSON(t *testing.T) {
	tool := &SearchTool{}
	_, err := tool.Execute(context.Background(), tools.ToolCallContext{}, `invalid json`)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSearchToolNilServicesReturnsError(t *testing.T) {
	tool := &SearchTool{}
	out, err := tool.Execute(context.Background(), tools.ToolCallContext{}, `{"searches": [{"id": "kb1", "channel": "knowledge_base", "query": "test"}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty response even with nil knowledge service")
	}
}
