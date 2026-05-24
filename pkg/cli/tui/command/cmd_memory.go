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

package command

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
)

func handleMemoryCmd(b backend.Backend, bridge Bridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	items, err := b.ListMemories(agentID)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to list memories: %w", err)
	}

	if len(items) == 0 {
		bridge.Info("No memories found for this agent")
		return CommandResult{Handled: true}, nil
	}

	var sb strings.Builder
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "  %-10s %-24s %-62s %s\n",
		styleGray.Render("ID"),
		styleGray.Render("Title"),
		styleGray.Render("Preview"),
		styleGray.Render("Created At"))
	sb.WriteString(renderTableSeparator())
	for _, item := range items {
		title := item.Title
		if title == "" {
			title = "(untitled)"
		}
		preview := item.Preview
		if len(preview) > maxToolResultPreview {
			preview = preview[:maxToolResultPreview] + "..."
		}
		fmt.Fprintf(&sb, "  %-10s %-24s %-62s %s\n",
			item.ID.String()[:8],
			title,
			preview,
			item.CreatedAt.Format("2006-01-02 15:04"))
	}
	bridge.Print(sb.String())
	return CommandResult{Handled: true}, nil
}
