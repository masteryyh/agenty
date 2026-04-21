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
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/services"
)

type FindSkillTool struct {
	skillService *services.SkillService
}

func (t *FindSkillTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name:        "find_skill",
		Description: consts.FindSkillToolDescription,
		Parameters: tools.ToolParameters{
			Type: "object",
			Properties: map[string]tools.ParameterProperty{
				"query": {
					Type:        "string",
					Description: "Search query to find matching skills. Can be a skill name, keyword, or description fragment.",
				},
			},
			Required: []string{"query"},
		},
	}
}

func (t *FindSkillTool) Execute(ctx context.Context, tcc tools.ToolCallContext, arguments string) (string, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if strings.TrimSpace(args.Query) == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	sessionID := tcc.SessionID
	results, err := t.skillService.SearchSkills(ctx, &sessionID, args.Query, 20)
	if err != nil {
		return "", fmt.Errorf("failed to search skills: %w", err)
	}

	if len(results) == 0 {
		return "No matching skills found for the given query.", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d skill(s):\n\n", len(results))

	for i, skill := range results {
		fmt.Fprintf(&sb, "%d. **%s**\n", i+1, skill.Name)
		fmt.Fprintf(&sb, "   Description: %s\n", truncateDescription(skill.Description, 200))
		if skill.Scope != "" {
			fmt.Fprintf(&sb, "   Scope: %s\n", skill.Scope)
		}
		fmt.Fprintf(&sb, "   Path: %s\n", skill.SkillMDPath)
		if skill.Score > 0 {
			fmt.Fprintf(&sb, "   Relevance: %.2f\n", skill.Score)
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func truncateDescription(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
