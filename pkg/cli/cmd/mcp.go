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
	"os"
	"strings"

	"github.com/masteryyh/agenty/pkg/models"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage MCP servers",
	Long:  `Register, list, update and delete MCP server connections`,
}

var mcpTransportOptions = []string{"stdio", "sse", "streamable-http"}

var mcpListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all MCP servers",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()

		page, _ := cmd.Flags().GetInt("page")
		pageSize, _ := cmd.Flags().GetInt("page-size")

		result, err := c.ListMCPServers(page, pageSize)
		if err != nil {
			return err
		}

		if len(result.Data) == 0 {
			pterm.Warning.Println("No MCP servers found")
			return nil
		}

		tableData := pterm.TableData{
			{"Name", "Transport", "Enabled", "Target", "Status"},
		}
		for _, s := range result.Data {
			target := s.URL
			if s.Transport == models.MCPTransportStdio {
				target = s.Command
			}
			enabled := "✓"
			if !s.Enabled {
				enabled = "✗"
			}
			status := s.Status
			if status == "" {
				status = "-"
			}
			tableData = append(tableData, []string{s.Name, string(s.Transport), enabled, target, status})
		}
		pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
		pterm.Info.Printf("Total: %d, Page: %d/%d\n", result.Total, result.Page, (result.Total+int64(result.PageSize)-1)/int64(result.PageSize))
		return nil
	},
}

var mcpCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Register a new MCP server",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("must be run in an interactive terminal")
		}

		c := GetClient()

		name, err := pterm.DefaultInteractiveTextInput.Show("Server name")
		if err != nil {
			return err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("name cannot be empty")
		}

		selectedTransport, err := pterm.DefaultInteractiveSelect.
			WithOptions(mcpTransportOptions).
			Show("Transport type")
		if err != nil {
			return err
		}

		dto := &models.CreateMCPServerDto{
			Name:      name,
			Transport: models.MCPTransportType(selectedTransport),
		}

		switch models.MCPTransportType(selectedTransport) {
		case models.MCPTransportStdio:
			command, err := pterm.DefaultInteractiveTextInput.Show("Command (e.g., npx, node, python)")
			if err != nil {
				return err
			}
			dto.Command = strings.TrimSpace(command)

			argsStr, err := pterm.DefaultInteractiveTextInput.Show("Arguments (space-separated, leave empty for none)")
			if err != nil {
				return err
			}
			argsStr = strings.TrimSpace(argsStr)
			if argsStr != "" {
				dto.Args = strings.Fields(argsStr)
			}

		case models.MCPTransportSSE, models.MCPTransportStreamableHTTP:
			url, err := pterm.DefaultInteractiveTextInput.Show("Server URL")
			if err != nil {
				return err
			}
			dto.URL = strings.TrimSpace(url)
		}

		server, err := c.CreateMCPServer(dto)
		if err != nil {
			return err
		}
		pterm.Success.Printf("MCP server registered: %s\n", server.Name)

		autoConnect, err := pterm.DefaultInteractiveConfirm.Show("Connect now?")
		if err != nil {
			return err
		}
		if autoConnect {
			if err := c.ConnectMCPServer(server.ID); err != nil {
				pterm.Warning.Printf("Connection failed: %s\n", err)
			} else {
				pterm.Success.Println("Connected")
			}
		}

		return nil
	},
}

func mcpServerLabel(s models.MCPServerDto) string {
	target := s.URL
	if s.Transport == models.MCPTransportStdio {
		target = s.Command
	}
	return fmt.Sprintf("%s (%s → %s)", s.Name, s.Transport, target)
}

var mcpUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update an MCP server",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("must be run in an interactive terminal")
		}

		c := GetClient()

		result, err := c.ListMCPServers(1, 100)
		if err != nil {
			return err
		}
		if len(result.Data) == 0 {
			pterm.Warning.Println("No MCP servers found")
			return nil
		}

		options := make([]string, len(result.Data))
		for i, s := range result.Data {
			options[i] = mcpServerLabel(s)
		}
		selected, err := pterm.DefaultInteractiveSelect.WithOptions(options).Show("Select server to update")
		if err != nil {
			return err
		}
		var target models.MCPServerDto
		for _, s := range result.Data {
			if mcpServerLabel(s) == selected {
				target = s
				break
			}
		}

		pterm.Info.Println("Press Enter to keep the current value")

		newName, err := pterm.DefaultInteractiveTextInput.WithDefaultValue(target.Name).Show("Name")
		if err != nil {
			return err
		}
		newName = strings.TrimSpace(newName)

		enabledStr, err := pterm.DefaultInteractiveSelect.
			WithOptions([]string{"true", "false"}).
			WithDefaultOption(fmt.Sprintf("%t", target.Enabled)).
			Show("Enabled")
		if err != nil {
			return err
		}
		enabled := enabledStr == "true"

		dto := &models.UpdateMCPServerDto{
			Name:    newName,
			Enabled: &enabled,
		}

		switch target.Transport {
		case models.MCPTransportStdio:
			newCmd, err := pterm.DefaultInteractiveTextInput.WithDefaultValue(target.Command).Show("Command")
			if err != nil {
				return err
			}
			dto.Command = strings.TrimSpace(newCmd)
		case models.MCPTransportSSE, models.MCPTransportStreamableHTTP:
			newURL, err := pterm.DefaultInteractiveTextInput.WithDefaultValue(target.URL).Show("URL")
			if err != nil {
				return err
			}
			dto.URL = strings.TrimSpace(newURL)
		}

		updated, err := c.UpdateMCPServer(target.ID, dto)
		if err != nil {
			return err
		}
		pterm.Success.Printf("MCP server updated: %s\n", updated.Name)
		return nil
	},
}

var mcpDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete an MCP server",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("must be run in an interactive terminal")
		}

		c := GetClient()

		result, err := c.ListMCPServers(1, 100)
		if err != nil {
			return err
		}
		if len(result.Data) == 0 {
			pterm.Warning.Println("No MCP servers found")
			return nil
		}

		options := make([]string, len(result.Data))
		for i, s := range result.Data {
			options[i] = mcpServerLabel(s)
		}
		selected, err := pterm.DefaultInteractiveSelect.WithOptions(options).Show("Select server to delete")
		if err != nil {
			return err
		}
		var target models.MCPServerDto
		for _, s := range result.Data {
			if mcpServerLabel(s) == selected {
				target = s
				break
			}
		}

		confirm, err := pterm.DefaultInteractiveConfirm.Show(fmt.Sprintf("Delete MCP server '%s'?", target.Name))
		if err != nil {
			return err
		}
		if !confirm {
			pterm.Info.Println("Cancelled")
			return nil
		}

		if err := c.DeleteMCPServer(target.ID); err != nil {
			return err
		}
		pterm.Success.Printf("MCP server deleted: %s\n", target.Name)
		return nil
	},
}

var mcpStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show MCP server connection status",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()

		result, err := c.ListMCPServers(1, 100)
		if err != nil {
			return err
		}

		if len(result.Data) == 0 {
			pterm.Warning.Println("No MCP servers found")
			return nil
		}

		for _, s := range result.Data {
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

			pterm.Printf("%s %s (%s)\n", statusIcon, s.Name, s.Transport)
			if len(s.Tools) > 0 {
				pterm.Printf("   Tools: %s\n", strings.Join(s.Tools, ", "))
			}
			if s.Error != "" {
				pterm.Printf("   Error: %s\n", s.Error)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)

	mcpCmd.AddCommand(mcpListCmd)
	mcpListCmd.Flags().Int("page", 1, "Page number")
	mcpListCmd.Flags().Int("page-size", 10, "Page size")

	mcpCmd.AddCommand(mcpCreateCmd)
	mcpCmd.AddCommand(mcpUpdateCmd)
	mcpCmd.AddCommand(mcpDeleteCmd)
	mcpCmd.AddCommand(mcpStatusCmd)
}
