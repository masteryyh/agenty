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

	"github.com/charmbracelet/huh"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/models"
)

var mcpTransportOptions = []string{"stdio", "sse", "streamable-http"}

func mcpServerLabel(s models.MCPServerDto) string {
	target := s.URL
	if s.Transport == models.MCPTransportStdio {
		target = s.Command
	}
	return fmt.Sprintf("%s (%s → %s)", s.Name, s.Transport, target)
}

func handleMCPCmd(b backend.Backend, bridge *UIBridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	for {
		result, err := b.ListMCPServers(1, 100)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to list MCP servers: %w", err)
		}

		if len(result.Data) == 0 {
			res, err := bridge.ShowList("MCP Servers", []string{"(no servers)"}, "a add  ·  Esc back")
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if res.Action == ListActionAdd {
				if err := doCreateMCPServer(b, bridge); err != nil && !errors.Is(err, ErrCancelled) {
					bridge.Error("Failed to register MCP server: %v", err)
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
			items[i] = fmt.Sprintf("%s %s  %s  enabled:%s", statusIcon, mcpServerLabel(s), styleGray.Render(s.Status), enabled)
		}

		res, err := bridge.ShowListWithCursorAndActions("MCP Servers", items, listHints, 0, nil, mcpDeleteConfirm(result.Data))
		if err != nil {
			return CommandResult{Handled: true}, err
		}

		switch res.Action {
		case ListActionSelect:
			if err := doUpdateMCPServer(b, bridge, result.Data[res.Index]); err != nil && !errors.Is(err, ErrCancelled) {
				bridge.Error("Failed to update MCP server: %v", err)
			}
			continue

		case ListActionAdd:
			if err := doCreateMCPServer(b, bridge); err != nil && !errors.Is(err, ErrCancelled) {
				bridge.Error("Failed to register MCP server: %v", err)
			}
			continue

		case ListActionEdit:
			if err := doUpdateMCPServer(b, bridge, result.Data[res.Index]); err != nil && !errors.Is(err, ErrCancelled) {
				bridge.Error("Failed to update MCP server: %v", err)
			}
			continue

		case ListActionDelete:
			target := result.Data[res.Index]
			if err := b.DeleteMCPServer(target.ID); err != nil {
				bridge.Error("Failed to delete MCP server: %v", err)
			} else {
				bridge.Success("MCP server deleted: %s", target.Name)
			}
			continue

		case ListActionCancel:
			return CommandResult{Handled: true}, nil
		}
	}
}

func mcpDeleteConfirm(serverList []models.MCPServerDto) func(idx int) string {
	return func(idx int) string {
		if idx < 0 || idx >= len(serverList) {
			return ""
		}
		return fmt.Sprintf("Delete MCP server '%s'?", serverList[idx].Name)
	}
}

func doCreateMCPServer(b backend.Backend, bridge *UIBridge) error {
	var name, selectedTransport, command, argsStr, serverURL string
	selectedTransport = mcpTransportOptions[0]

	transportOpts := make([]huh.Option[string], len(mcpTransportOptions))
	for i, t := range mcpTransportOptions {
		transportOpts[i] = huh.NewOption(t, t)
	}

	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Name").Value(&name),
		huh.NewSelect[string]().Title("Transport").Options(transportOpts...).Value(&selectedTransport),
		huh.NewInput().Title("Command").Placeholder("stdio only — executable path or name").Value(&command),
		huh.NewInput().Title("Arguments").Placeholder("stdio only — space-separated args, leave blank for none").Value(&argsStr),
		huh.NewInput().Title("Server URL").Placeholder("sse/streamable-http only — e.g. http://localhost:3000/sse").Value(&serverURL),
	))

	submitted, err := bridge.ShowValidatedHuhForm(form, func() error {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("Name is required.")
		}
		switch models.MCPTransportType(selectedTransport) {
		case models.MCPTransportStdio:
			if strings.TrimSpace(command) == "" {
				return fmt.Errorf("Command is required for stdio transport.")
			}
		case models.MCPTransportSSE, models.MCPTransportStreamableHTTP:
			if strings.TrimSpace(serverURL) == "" {
				return fmt.Errorf("Server URL is required for %s transport.", selectedTransport)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !submitted {
		return ErrCancelled
	}

	transport := models.MCPTransportType(selectedTransport)
	dto := &models.CreateMCPServerDto{
		Name:      strings.TrimSpace(name),
		Transport: transport,
	}

	switch transport {
	case models.MCPTransportStdio:
		if strings.TrimSpace(command) == "" {
			return fmt.Errorf("command is required for stdio transport")
		}
		dto.Command = strings.TrimSpace(command)
		if a := strings.TrimSpace(argsStr); a != "" {
			dto.Args = strings.Fields(a)
		}
	case models.MCPTransportSSE, models.MCPTransportStreamableHTTP:
		if strings.TrimSpace(serverURL) == "" {
			return fmt.Errorf("server URL is required for %s transport", transport)
		}
		dto.URL = strings.TrimSpace(serverURL)
	}

	server, err := b.CreateMCPServer(dto)
	if err != nil {
		return err
	}
	bridge.Success("MCP server registered: %s", server.Name)

	autoConnect, err := bridge.ShowConfirm("Connect now?")
	if err != nil {
		return err
	}
	if autoConnect {
		if err := b.ConnectMCPServer(server.ID); err != nil {
			bridge.Warning("Connection failed: %s", err)
		} else {
			bridge.Success("Connected")
		}
	}

	return nil
}

func doUpdateMCPServer(b backend.Backend, bridge *UIBridge, target models.MCPServerDto) error {
	newName := target.Name
	enabled := target.Enabled

	var fields []huh.Field
	fields = append(fields,
		huh.NewInput().Title("Name").Value(&newName),
		huh.NewSelect[bool]().Title("Enabled").
			Options(huh.NewOption("Yes", true), huh.NewOption("No", false)).
			Value(&enabled),
	)

	var newCommand, newURL string
	switch target.Transport {
	case models.MCPTransportStdio:
		newCommand = target.Command
		fields = append(fields, huh.NewInput().Title("Command").Value(&newCommand))
	case models.MCPTransportSSE, models.MCPTransportStreamableHTTP:
		newURL = target.URL
		fields = append(fields, huh.NewInput().Title("URL").Value(&newURL))
	}

	form := huh.NewForm(huh.NewGroup(fields...))
	submitted, err := bridge.ShowValidatedHuhForm(form, func() error {
		if strings.TrimSpace(newName) == "" {
			return fmt.Errorf("Name is required.")
		}
		switch target.Transport {
		case models.MCPTransportStdio:
			if strings.TrimSpace(newCommand) == "" {
				return fmt.Errorf("Command is required for stdio transport.")
			}
		case models.MCPTransportSSE, models.MCPTransportStreamableHTTP:
			if strings.TrimSpace(newURL) == "" {
				return fmt.Errorf("URL is required for %s transport.", target.Transport)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !submitted {
		return ErrCancelled
	}

	if newName == "" {
		newName = target.Name
	}
	dto := &models.UpdateMCPServerDto{
		Name:    newName,
		Enabled: &enabled,
	}

	switch target.Transport {
	case models.MCPTransportStdio:
		if newCommand == "" {
			newCommand = target.Command
		}
		dto.Command = newCommand
	case models.MCPTransportSSE, models.MCPTransportStreamableHTTP:
		if newURL == "" {
			newURL = target.URL
		}
		dto.URL = newURL
	}

	updated, err := b.UpdateMCPServer(target.ID, dto)
	if err != nil {
		return err
	}
	bridge.Success("MCP server updated: %s", updated.Name)
	return nil
}
