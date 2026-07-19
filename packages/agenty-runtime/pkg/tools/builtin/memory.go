package builtin

import (
	"context"
	"fmt"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/tools"
)

type SaveMemoryTool struct {
	knowledgeService *services.KnowledgeService
}

func (t *SaveMemoryTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name:        "save_memory",
		Description: "Save a piece of information to long-term memory for future reference and return JSON with the saved item id. Use this to remember important facts, user preferences, or key information from conversations.",
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

func (t *SaveMemoryTool) Execute(ctx context.Context, tcc tools.ToolCallContext, arguments string) (string, error) {
	var args struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if strings.TrimSpace(args.Content) == "" {
		return "", fmt.Errorf("content cannot be empty")
	}

	result, err := t.knowledgeService.CreateItemSync(ctx, tcc.AgentID, &models.CreateKnowledgeItemDto{
		Category: models.KnowledgeCategoryLLMMemory,
		Title:    truncateForTitle(args.Content),
		Content:  args.Content,
	})
	if err != nil {
		return "", fmt.Errorf("failed to save memory: %w", err)
	}

	return marshalToolResult(map[string]any{
		"id":       result.ID,
		"saved":    true,
		"category": models.KnowledgeCategoryLLMMemory,
	})
}

func truncateForTitle(s string) string {
	if len(s) <= 100 {
		return s
	}
	return s[:97] + "..."
}
