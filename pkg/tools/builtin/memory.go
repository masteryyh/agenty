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

	return fmt.Sprintf("Memory saved successfully with ID: %s", result.ID), nil
}

func truncateForTitle(s string) string {
	if len(s) <= 100 {
		return s
	}
	return s[:97] + "..."
}
