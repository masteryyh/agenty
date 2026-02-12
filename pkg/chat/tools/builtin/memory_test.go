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
