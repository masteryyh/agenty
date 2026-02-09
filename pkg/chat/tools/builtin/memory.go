package builtin

import (
	"context"
	"fmt"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/services"
)

type SaveMemoryTool struct {
	memoryService *services.MemoryService
}

func (t *SaveMemoryTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name:        "save_memory",
		Description: "Save a piece of information to long-term memory for future reference. Use this to remember important facts, user preferences, or key information from conversations.",
		Parameters: tools.ToolParameters{
			Type: "object",
			Properties: map[string]tools.ParameterProperty{
				"content": {
					Type:        "string",
					Description: "The information to save to memory. Should be a clear, concise statement of the fact or information to remember.",
				},
			},
			Required: []string{"content"},
		},
	}
}

func (t *SaveMemoryTool) Execute(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if strings.TrimSpace(args.Content) == "" {
		return "", fmt.Errorf("content cannot be empty")
	}

	result, err := t.memoryService.SaveMemory(ctx, args.Content)
	if err != nil {
		return "", fmt.Errorf("failed to save memory: %w", err)
	}

	return fmt.Sprintf("Memory saved successfully with ID: %s", result.ID), nil
}

type SearchMemoryTool struct {
	memoryService *services.MemoryService
}

func (t *SearchMemoryTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name:        "search_memory",
		Description: "Search long-term memory for relevant information. Uses semantic search, full-text search, and keyword matching to find the most relevant memories.",
		Parameters: tools.ToolParameters{
			Type: "object",
			Properties: map[string]tools.ParameterProperty{
				"query": {
					Type:        "string",
					Description: "The search query to find relevant memories",
				},
			},
			Required: []string{"query"},
		},
	}
}

func (t *SearchMemoryTool) Execute(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if strings.TrimSpace(args.Query) == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	results, err := t.memoryService.SearchMemory(ctx, args.Query, 5)
	if err != nil {
		return "", fmt.Errorf("failed to search memory: %w", err)
	}

	if len(results) == 0 {
		return "No relevant memories found.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d relevant memories:\n\n", len(results)))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. [Score: %.4f] %s\n", i+1, r.Score, r.Memory.Content))
	}
	return sb.String(), nil
}
