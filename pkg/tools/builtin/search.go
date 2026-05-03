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
	"fmt"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/tools"
)

type SearchTool struct {
	searchService *services.SearchService
}

func (t *SearchTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name:        "search",
		Description: consts.SearchToolDescription,
		Parameters: tools.ToolParameters{
			Type: "object",
			Properties: map[string]tools.ParameterProperty{
				"searches": {
					Type:        "array",
					Description: `Array of search specs. Each element specifies one search operation.`,
					Items: &tools.ParameterProperty{
						Type: "object",
						Properties: map[string]tools.ParameterProperty{
							"id": {
								Type:        "string",
								Description: `Unique identifier for this search spec (e.g., "kb_kw", "file1", "web1"). Returned with results to identify which spec produced each result.`,
							},
							"channel": {
								Type:        "string",
								Description: `Search channel: "knowledge_base", "workspace_files", or "web_search".`,
							},
							"query": {
								Type:        "string",
								Description: `Search query string tailored to the channel and strategy (see tool description for format guidance).`,
							},
							"limit": {
								Type:        "integer",
								Description: `Maximum number of results to return for this spec. Defaults to 10.`,
							},
							"includeGlobs": {
								Type:        "array",
								Description: `Optional glob filters for workspace_files relative paths. Example: ["*.go", "pkg/*"].`,
								Items:       &tools.ParameterProperty{Type: "string"},
							},
							"excludeGlobs": {
								Type:        "array",
								Description: `Optional glob filters to exclude workspace_files relative paths.`,
								Items:       &tools.ParameterProperty{Type: "string"},
							},
						},
						Required: []string{"id", "channel", "query"},
					},
				},
			},
			Required: []string{"searches"},
		},
	}
}

func (t *SearchTool) Execute(ctx context.Context, tcc tools.ToolCallContext, arguments string) (string, error) {
	var req models.SearchRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if err := models.ValidateSearchRequest(req); err != nil {
		return "", err
	}

	searchService := t.searchService
	if searchService == nil {
		searchService = services.GetSearchService()
	}

	resp, err := searchService.Search(ctx, models.SearchContext{
		AgentID:   tcc.AgentID,
		SessionID: tcc.SessionID,
		ModelID:   tcc.ModelID,
		ModelCode: tcc.ModelCode,
		Cwd:       tcc.Cwd,
	}, req)
	if err != nil {
		return "", err
	}

	out, err := json.MarshalString(resp)
	if err != nil {
		return "", fmt.Errorf("failed to serialize search response: %w", err)
	}
	return out, nil
}
