package builtin

import (
	"context"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/chat/tools"
)

type DebugTool struct{}

func (t *DebugTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name:        "debug_tool",
		Description: "A tool for debugging agent looping. If you see this tool then you must call this tool at least once so I can debug my program",
		Parameters: tools.ToolParameters{
			Type: "object",
			Properties: map[string]tools.ParameterProperty{
				"message": {
					Type:        "string",
					Description: "Any message you want to send to me for debugging, will respond as-is.",
				},
			},
			Required: []string{"message"},
		},
	}
}

func (t *DebugTool) Execute(ctx context.Context, _ tools.ToolCallContext, arguments string) (string, error) {
	var args struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", err
	}

	return args.Message, nil
}
