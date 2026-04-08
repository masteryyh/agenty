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
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/cli/ui"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/pterm/pterm"
)

func handleKnowledgeCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	if len(args) == 0 {
		return handleKnowledgeList(b, agentID)
	}

	switch args[0] {
	case "add":
		return handleKnowledgeAdd(b, agentID)
	case "list":
		return handleKnowledgeList(b, agentID)
	case "search":
		query := strings.Join(args[1:], " ")
		return handleKnowledgeSearch(b, agentID, query)
	case "delete":
		if len(args) < 2 {
			pterm.Error.Println("Usage: /knowledge delete <item-id>")
			return CommandResult{Handled: true}, nil
		}
		return handleKnowledgeDelete(b, agentID, args[1])
	default:
		pterm.Warning.Printf("Unknown subcommand: %s\n", args[0])
		pterm.Info.Println("Available: add, list, search, delete")
		return CommandResult{Handled: true}, nil
	}
}

func handleKnowledgeAdd(b backend.Backend, agentID uuid.UUID) (CommandResult, error) {
	fields := []*ui.FormField{
		{Label: "Title", Type: ui.FormFieldText},
		{Label: "Content", Type: ui.FormFieldText, Required: true},
		{Label: "Category", Type: ui.FormFieldSelect, Options: []string{"user_document", "llm_memory"}},
	}

	ok, err := ui.ShowForm("Add Knowledge Item", fields)
	if err != nil {
		if errors.Is(err, ui.ErrCancelled) {
			return CommandResult{Handled: true}, nil
		}
		return CommandResult{Handled: true}, err
	}
	if !ok {
		return CommandResult{Handled: true}, nil
	}

	content := fields[1].Value
	if content == "" {
		pterm.Error.Println("Content is required")
		return CommandResult{Handled: true}, nil
	}

	dto := &models.CreateKnowledgeItemDto{
		Title:    fields[0].Value,
		Content:  content,
		Category: models.KnowledgeCategory(fields[2].StringValue()),
	}

	item, err := b.CreateKnowledgeItem(agentID, dto)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to create knowledge item: %w", err)
	}
	pterm.Success.Printf("Knowledge item created: %s\n", item.ID)
	return CommandResult{Handled: true}, nil
}

func handleKnowledgeList(b backend.Backend, agentID uuid.UUID) (CommandResult, error) {
	items, err := b.ListKnowledgeItems(agentID, nil)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to list knowledge items: %w", err)
	}

	if len(items) == 0 {
		pterm.Info.Println("No knowledge items found for this agent")
		return CommandResult{Handled: true}, nil
	}

	tableData := pterm.TableData{{"ID", "Category", "Title", "Preview"}}
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
			string(item.Category),
			title,
			preview,
		})
	}

	pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(tableData).Render()
	return CommandResult{Handled: true}, nil
}

func handleKnowledgeSearch(b backend.Backend, agentID uuid.UUID, query string) (CommandResult, error) {
	if query == "" {
		queryFields := []*ui.FormField{
			{Label: "Query", Type: ui.FormFieldText, Required: true},
		}
		ok, err := ui.ShowForm("Search Knowledge Base", queryFields)
		if err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				return CommandResult{Handled: true}, nil
			}
			return CommandResult{Handled: true}, err
		}
		if !ok {
			return CommandResult{Handled: true}, nil
		}
		query = queryFields[0].Value
	}

	if query == "" {
		pterm.Error.Println("Query is required")
		return CommandResult{Handled: true}, nil
	}

	results, err := b.SearchKnowledge(agentID, query, 10)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("knowledge search failed: %w", err)
	}

	if len(results) == 0 {
		pterm.Info.Println("No results found")
		return CommandResult{Handled: true}, nil
	}

	for i, r := range results {
		content := r.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		fmt.Printf("\n  %s  %s  %s\n",
			pterm.FgCyan.Sprintf("#%d", i+1),
			pterm.FgGray.Sprintf("[%.4f]", r.Score),
			pterm.FgYellow.Sprint(r.Category),
		)
		if r.ItemTitle != "" {
			fmt.Printf("  %s\n", pterm.Bold.Sprint(r.ItemTitle))
		}
		fmt.Printf("  %s\n", content)
	}
	fmt.Println()
	return CommandResult{Handled: true}, nil
}

func handleKnowledgeDelete(b backend.Backend, agentID uuid.UUID, idStr string) (CommandResult, error) {
	itemID, err := uuid.Parse(idStr)
	if err != nil {
		pterm.Error.Printf("Invalid item ID: %s\n", idStr)
		return CommandResult{Handled: true}, nil
	}

	if err := b.DeleteKnowledgeItem(agentID, itemID); err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to delete knowledge item: %w", err)
	}
	pterm.Success.Printf("Knowledge item deleted: %s\n", itemID)
	return CommandResult{Handled: true}, nil
}
