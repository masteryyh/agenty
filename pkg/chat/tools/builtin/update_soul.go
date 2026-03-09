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
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/services"
)

type UpdateSoulTool struct {
	agentService *services.AgentService
}

func (t *UpdateSoulTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name:        "update_soul",
		Description: "Update the agent's own soul (personality and character). Call this when the user explicitly requests changes to the agent's personality, character traits, tone, or behavioral style.",
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

func (t *UpdateSoulTool) Execute(ctx context.Context, agentID uuid.UUID, arguments string) (string, error) {
	var args struct {
		Soul string `json:"soul"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if agentID == uuid.Nil {
		return "", fmt.Errorf("agent ID is required")
	}

	if strings.TrimSpace(args.Soul) == "" {
		return "", fmt.Errorf("soul cannot be empty")
	}

	if err := t.agentService.UpdateAgent(ctx, agentID, &models.UpdateAgentDto{Soul: &args.Soul}); err != nil {
		return "", fmt.Errorf("failed to update soul: %w", err)
	}

	return "Soul updated successfully.", nil
}
