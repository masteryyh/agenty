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

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/chat/tools"
)

type TodoTool struct{}

func (t *TodoTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name: "todo",
		Description: "Manage a per-session task planning list. Actions: " +
			"'add' — append items (requires items[]); " +
			"'update' — change status of one item by id (requires id, status: pending|in_progress|done); " +
			"'list' — show all items with status summary.",
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
		return mgr.Add(tcc.SessionID, args.Items)
	case "update":
		return mgr.Update(tcc.SessionID, args.ID, args.Status)
	case "list":
		return mgr.FormatList(tcc.SessionID), nil
	default:
		return "", fmt.Errorf("unknown action %q: must be 'add', 'update', or 'list'", args.Action)
	}
}
