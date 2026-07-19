package builtin

import (
	"context"
	"fmt"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/tools"
)

type UpdateSoulTool struct {
	agentService *services.AgentService
}

func (t *UpdateSoulTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name:        "update_soul",
		Description: "Update the agent's own soul (personality and character) and return JSON. Call this when the user explicitly requests changes to the agent's personality, character traits, tone, or behavioral style.",
		Parameters: tools.ToolParameters{
			Type: "object",
			Properties: map[string]tools.ParameterProperty{
				"soul": {
					Type:        "string",
					Description: "The new soul content describing the agent's personality, character, tone, and behavioral style.",
				},
			},
			Required: []string{"soul"},
		},
	}
}

func (t *UpdateSoulTool) Execute(ctx context.Context, tcc tools.ToolCallContext, arguments string) (string, error) {
	var args struct {
		Soul string `json:"soul"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if tcc.AgentID == uuid.Nil {
		return "", fmt.Errorf("agent ID is required")
	}

	if strings.TrimSpace(args.Soul) == "" {
		return "", fmt.Errorf("soul cannot be empty")
	}

	if err := t.agentService.UpdateAgent(ctx, tcc.AgentID, &models.UpdateAgentDto{Soul: &args.Soul}); err != nil {
		return "", fmt.Errorf("failed to update soul: %w", err)
	}

	return marshalToolResult(map[string]any{
		"updated": true,
	})
}
