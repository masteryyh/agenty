package builtin

import (
	"context"
	"fmt"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/tools"
)

type TodoTool struct{}

func (t *TodoTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name: "todo",
		Description: "Manage a per-session task planning list. Actions: " +
			"'add' — append items (requires items[]); " +
			"'update' — change status of one item by id (requires id, status: pending|in_progress|done); " +
			"'list' — show all items with status summary. Returns JSON.",
		Parameters: tools.ToolParameters{
			Type: "object",
			Properties: map[string]tools.ParameterProperty{
				"action": {
					Type:        "string",
					Description: "Operation to perform: 'add', 'update', or 'list'",
				},
				"items": {
					Type:        "array",
					Description: "Required for 'add': list of task description strings to append",
					Items:       &tools.ParameterProperty{Type: "string"},
				},
				"id": {
					Type:        "integer",
					Description: "Required for 'update': numeric ID of the item to modify",
				},
				"status": {
					Type:        "string",
					Description: "Required for 'update': new status — 'pending', 'in_progress', or 'done'",
				},
			},
			Required: []string{"action"},
		},
	}
}

func (t *TodoTool) Execute(_ context.Context, tcc tools.ToolCallContext, arguments string) (string, error) {
	var args struct {
		Action string   `json:"action"`
		Items  []string `json:"items,omitempty"`
		ID     int      `json:"id,omitempty"`
		Status string   `json:"status,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if tcc.SessionID == uuid.Nil {
		return "", fmt.Errorf("session ID is required for todo tool")
	}

	mgr := tools.GetTodoManager()
	switch args.Action {
	case "add":
		items, err := mgr.AddItems(tcc.SessionID, args.Items)
		if err != nil {
			return "", err
		}
		return marshalToolResult(map[string]any{
			"action": "add",
			"count":  len(items),
			"items":  items,
		})
	case "update":
		item, err := mgr.UpdateStatus(tcc.SessionID, args.ID, args.Status)
		if err != nil {
			return "", err
		}
		return marshalToolResult(map[string]any{
			"action": "update",
			"item":   item,
		})
	case "list":
		items := mgr.List(tcc.SessionID)
		if items == nil {
			items = []models.TodoItemDto{}
		}
		pending, inProgress, done := 0, 0, 0
		for _, item := range items {
			switch item.Status {
			case "pending":
				pending++
			case "in_progress":
				inProgress++
			case "done":
				done++
			}
		}
		return marshalToolResult(map[string]any{
			"action":     "list",
			"items":      items,
			"total":      len(items),
			"pending":    pending,
			"inProgress": inProgress,
			"done":       done,
		})
	default:
		return "", fmt.Errorf("unknown action %q: must be 'add', 'update', or 'list'", args.Action)
	}
}
