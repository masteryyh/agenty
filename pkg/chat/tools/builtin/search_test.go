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
	"os"
	"path/filepath"
	"strings"
	"testing"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/services"
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
	tool := &SearchTool{searchService: &services.SearchService{}}
	out, err := tool.Execute(context.Background(), tools.ToolCallContext{}, `{"searches": [{"id": "kb1", "channel": "knowledge_base", "query": "test"}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty response even with nil knowledge service")
	}
}

func TestSearchToolWorkspaceFilesRequiresCwd(t *testing.T) {
	tool := &SearchTool{searchService: &services.SearchService{}}
	out, err := tool.Execute(context.Background(), tools.ToolCallContext{}, `{"searches": [{"id": "file1", "channel": "workspace_files", "query": "search tool"}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"quality":"error"`) {
		t.Fatalf("expected workspace_files error quality, got %s", out)
	}
}

func TestSearchToolWorkspaceFilesFindsContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pkg", "search.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "package pkg\n\nfunc SearchToolExample() string {\n\treturn \"workspace file search target\"\n}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := &SearchTool{searchService: &services.SearchService{}}
	out, err := tool.Execute(context.Background(), tools.ToolCallContext{Cwd: dir}, `{"searches": [{"id": "file1", "channel": "workspace_files", "query": "workspace file search target", "limit": 5}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp struct {
		WorkspaceFiles *struct {
			Quality string `json:"quality"`
			Results []struct {
				RelativePath string `json:"relativePath"`
				Content      string `json:"content"`
			} `json:"results"`
		} `json:"workspaceFiles"`
		RankedResults []struct {
			Channel string `json:"channel"`
		} `json:"rankedResults"`
	}
	if err := json.UnmarshalString(out, &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.WorkspaceFiles == nil || resp.WorkspaceFiles.Quality == "no_results" || len(resp.WorkspaceFiles.Results) == 0 {
		t.Fatalf("expected workspace file results, got %s", out)
	}
	if resp.WorkspaceFiles.Results[0].RelativePath != "pkg/search.go" {
		t.Fatalf("unexpected relative path: %s", resp.WorkspaceFiles.Results[0].RelativePath)
	}
	if len(resp.RankedResults) == 0 || resp.RankedResults[0].Channel != "workspace_files" {
		t.Fatalf("expected workspace_files ranked result, got %s", out)
	}
}
