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

var mcpTransportOptions = []string{"stdio", "sse", "streamable-http"}

func mcpServerLabel(s models.MCPServerDto) string {
	target := s.URL
	if s.Transport == models.MCPTransportStdio {
		target = s.Command
	}
	return fmt.Sprintf("%s (%s → %s)", s.Name, s.Transport, target)
}

func handleMCPCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	for {
		result, err := b.ListMCPServers(1, 100)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to list MCP servers: %w", err)
		}

		if len(result.Data) == 0 {
			pterm.Warning.Println("No MCP servers found")
			res, err := ui.ShowList("MCP Servers", []string{"(no servers)"}, "a add  ·  Esc back")
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if res.Action == ui.ListActionAdd {
				if err := doCreateMCPServer(b); err != nil && !errors.Is(err, ui.ErrCancelled) {
					pterm.Error.Printf("Failed to register MCP server: %v\n", err)
				}
				continue
			}
			return CommandResult{Handled: true}, nil
		}

		items := make([]string, len(result.Data))
		for i, s := range result.Data {
			statusIcon := "⚪"
			switch s.Status {
			case "connected":
				statusIcon = "🟢"
			case "error":
				statusIcon = "🔴"
			case "connecting":
				statusIcon = "🟡"
			case "disconnected":
				statusIcon = "⚫"
			}
			enabled := "✓"
			if !s.Enabled {
				enabled = "✗"
			}
			items[i] = fmt.Sprintf("%s %s  %s  enabled:%s", statusIcon, mcpServerLabel(s), pterm.FgGray.Sprint(s.Status), enabled)
		}

		res, err := ui.ShowList("MCP Servers", items, listHints)
		if err != nil {
			return CommandResult{Handled: true}, err
		}

		switch res.Action {
		case ui.ListActionSelect:
			target := result.Data[res.Index]
			fmt.Println()
			fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Name"), target.Name)
			fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Transport"), string(target.Transport))
			fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Status"), target.Status)
			if target.Transport == models.MCPTransportStdio {
				fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Command"), target.Command)
			} else {
				fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("URL"), target.URL)
			}
			if len(target.Tools) > 0 {
				fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Tools"), strings.Join(target.Tools, "  ·  "))
			}
			if target.Error != "" {
				fmt.Printf("  %-16s %s\n", pterm.FgRed.Sprint("Error"), target.Error)
			}
			fmt.Println()
			continue

		case ui.ListActionAdd:
			if err := doCreateMCPServer(b); err != nil && !errors.Is(err, ui.ErrCancelled) {
				pterm.Error.Printf("Failed to register MCP server: %v\n", err)
			}
			continue

		case ui.ListActionEdit:
			if err := doUpdateMCPServer(b, result.Data[res.Index]); err != nil && !errors.Is(err, ui.ErrCancelled) {
				pterm.Error.Printf("Failed to update MCP server: %v\n", err)
			}
			continue

		case ui.ListActionDelete:
			target := result.Data[res.Index]
			confirmed, err := ui.ShowConfirm(fmt.Sprintf("Delete MCP server '%s'?", target.Name))
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if confirmed {
				if err := b.DeleteMCPServer(target.ID); err != nil {
					pterm.Error.Printf("Failed to delete MCP server: %v\n", err)
				} else {
					pterm.Success.Printf("MCP server deleted: %s\n", target.Name)
				}
			}
			continue

		case ui.ListActionCancel:
			return CommandResult{Handled: true}, nil
		}
	}
}

func doCreateMCPServer(b backend.Backend) error {
	commandField := ui.TextField("Command", "", true)
	argsField := &ui.FormField{
		Label:       "Arguments",
		Type:        ui.FormFieldText,
		Placeholder: "space-separated, leave blank for none",
	}
	urlField := ui.TextField("Server URL", "", true)

	isStdio := func(fields []*ui.FormField) bool {
		return models.MCPTransportType(fields[1].Options[fields[1].SelIdx]) == models.MCPTransportStdio
	}
	isURLBased := func(fields []*ui.FormField) bool {
		t := models.MCPTransportType(fields[1].Options[fields[1].SelIdx])
		return t == models.MCPTransportSSE || t == models.MCPTransportStreamableHTTP
	}
	commandField.VisibleWhen = isStdio
	argsField.VisibleWhen = isStdio
	urlField.VisibleWhen = isURLBased

	fields := []*ui.FormField{
		ui.TextField("Name", "", true),
		ui.SelectField("Transport", mcpTransportOptions, 0),
		commandField,
		argsField,
		urlField,
	}

	submitted, err := ui.ShowForm("Register MCP Server", fields)
	if err != nil {
		return err
	}
	if !submitted {
		return ui.ErrCancelled
	}

	selectedTransport := models.MCPTransportType(mcpTransportOptions[fields[1].SelIdx])
	dto := &models.CreateMCPServerDto{
		Name:      fields[0].Value,
		Transport: selectedTransport,
	}
	switch selectedTransport {
	case models.MCPTransportStdio:
		dto.Command = commandField.Value
		if argsStr := argsField.Value; argsStr != "" {
			dto.Args = strings.Fields(argsStr)
		}
	case models.MCPTransportSSE, models.MCPTransportStreamableHTTP:
		dto.URL = urlField.Value
	}

	server, err := b.CreateMCPServer(dto)
	if err != nil {
		return err
	}
	pterm.Success.Printf("MCP server registered: %s\n", server.Name)

	autoConnect, err := ui.ShowConfirm("Connect now?")
	if err != nil {
		return err
	}
	if autoConnect {
		if err := b.ConnectMCPServer(server.ID); err != nil {
			pterm.Warning.Printf("Connection failed: %s\n", err)
		} else {
			pterm.Success.Println("Connected")
		}
	}

	return nil
}

func doUpdateMCPServer(b backend.Backend, target models.MCPServerDto) error {
	fields := []*ui.FormField{
		ui.TextField("Name", target.Name, false),
		ui.ToggleField("Enabled", target.Enabled),
	}

	switch target.Transport {
	case models.MCPTransportStdio:
		fields = append(fields, ui.TextField("Command", target.Command, false))
	case models.MCPTransportSSE, models.MCPTransportStreamableHTTP:
		fields = append(fields, ui.TextField("URL", target.URL, false))
	}

	submitted, err := ui.ShowForm("Update MCP Server", fields)
	if err != nil {
		return err
	}
	if !submitted {
		return ui.ErrCancelled
	}

	newName := fields[0].Value
	if newName == "" {
		newName = target.Name
	}
	enabled := fields[1].BoolValue()

	dto := &models.UpdateMCPServerDto{
		Name:    newName,
		Enabled: &enabled,
	}

	switch target.Transport {
	case models.MCPTransportStdio:
		newCmd := fields[2].Value
		if newCmd == "" {
			newCmd = target.Command
		}
		dto.Command = newCmd
	case models.MCPTransportSSE, models.MCPTransportStreamableHTTP:
		newURL := fields[2].Value
		if newURL == "" {
			newURL = target.URL
		}
		dto.URL = newURL
	}

	updated, err := b.UpdateMCPServer(target.ID, dto)
	if err != nil {
		return err
	}
	pterm.Success.Printf("MCP server updated: %s\n", updated.Name)
	return nil
}
