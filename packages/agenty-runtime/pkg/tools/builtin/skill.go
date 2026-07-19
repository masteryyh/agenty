package builtin

import (
	"context"
	"fmt"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/tools"
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
	if results == nil {
		results = []models.SkillSearchResult{}
	}

	return marshalToolResult(map[string]any{
		"query":   args.Query,
		"count":   len(results),
		"results": results,
	})
}
