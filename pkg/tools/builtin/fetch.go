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
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/tools"
)

type FetchTool struct {
	webFetchService *services.WebFetchService
}

func (t *FetchTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name:        "fetch",
		Description: consts.FetchToolDescription,
		Parameters: tools.ToolParameters{
			Type: "object",
			Properties: map[string]tools.ParameterProperty{
				"url": {
					Type:        "string",
					Description: "HTTP or HTTPS URL to fetch.",
				},
			},
			Required: []string{"url"},
		},
	}
}

func (t *FetchTool) Execute(ctx context.Context, _ tools.ToolCallContext, arguments string) (string, error) {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	args.URL = strings.TrimSpace(args.URL)
	if args.URL == "" {
		return "", fmt.Errorf("url cannot be empty")
	}

	webFetchService := t.webFetchService
	if webFetchService == nil {
		webFetchService = services.GetWebFetchService()
	}
	resp, err := webFetchService.Fetch(ctx, args.URL)
	if err != nil {
		return "", err
	}
	return marshalToolResult(resp)
}
