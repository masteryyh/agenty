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
