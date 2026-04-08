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

package cmd

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/pterm/pterm"
)

func handleMemoryCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	items, err := b.ListMemories(agentID)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to list memories: %w", err)
	}

	if len(items) == 0 {
		pterm.Info.Println("No memories found for this agent")
		return CommandResult{Handled: true}, nil
	}

	tableData := pterm.TableData{{"ID", "Title", "Preview", "Created At"}}
	for _, item := range items {
		title := item.Title
		if title == "" {
			title = "(untitled)"
		}
		preview := item.Preview
		if len(preview) > 60 {
			preview = preview[:60] + "..."
		}
		tableData = append(tableData, []string{
			item.ID.String()[:8],
			title,
			preview,
			item.CreatedAt.Format("2006-01-02 15:04"),
		})
	}

	pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(tableData).Render()
	return CommandResult{Handled: true}, nil
}
