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
	"sort"
	"strconv"
	"strings"

	"github.com/masteryyh/agenty/pkg/cli/actions"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/spf13/cobra"
)

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Manage MCP servers",
	}

	cmd.AddCommand(newMCPListCmd())
	cmd.AddCommand(newMCPGetCmd())
	cmd.AddCommand(newMCPAddCmd())
	cmd.AddCommand(newMCPUpdateCmd())
	cmd.AddCommand(newMCPRemoveCmd())
	cmd.AddCommand(newMCPConnectCmd())
	cmd.AddCommand(newMCPDisconnectCmd())

	return cmd
}

func newMCPListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List MCP servers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if page <= 0 || pageSize <= 0 || pageSize > 100 {
				return withExitCode(fmt.Errorf("--page must be >= 1 and --page-size must be between 1 and 100"), 2)
			}

			return runWithRuntime(cmd, true, false, func(runtime *Runtime) error {
				result, err := actions.ListMCPServers(runtime.Backend, page, pageSize)
				if err != nil {
					return err
				}
				if outputJSON {
					return writeJSON(cmd, result)
				}
				if result == nil || len(result.Data) == 0 {
					return writeLine(cmd, "No MCP servers.")
				}

				rows := make([][]string, 0, len(result.Data))
				for _, server := range result.Data {
					rows = append(rows, []string{
						server.ID.String(),
						server.Name,
						string(server.Transport),
						mcpTarget(server),
						strconv.FormatBool(server.Enabled),
						emptyOrDefault(server.Status, "disconnected"),
						strings.Join(server.Tools, ", "),
					})
				}

				if err := writeTable(cmd, []string{"ID", "Name", "Transport", "Target", "Enabled", "Status", "Tools"}, rows); err != nil {
					return err
				}
				return writeLine(cmd, "Page %d/%d  Total %d", result.Page, maxPage(result.Total, result.PageSize), result.Total)
			})
		},
	}
}

func newMCPGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name-or-id>",
		Short: "Show MCP server details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, true, false, func(runtime *Runtime) error {
				server, err := actions.GetMCPServer(runtime.Backend, args[0])
				if err != nil {
					return err
				}
				if outputJSON {
					return writeJSON(cmd, server)
				}

				rows := [][2]string{
					{"ID", server.ID.String()},
					{"Name", server.Name},
					{"Transport", string(server.Transport)},
					{"Enabled", strconv.FormatBool(server.Enabled)},
					{"Status", emptyOrDefault(server.Status, "disconnected")},
					{"Target", mcpTarget(*server)},
				}
				if server.Command != "" {
					rows = append(rows, [2]string{"Command", server.Command})
				}
				if len(server.Args) > 0 {
					rows = append(rows, [2]string{"Args", strings.Join(server.Args, " ")})
				}
				if len(server.Env) > 0 {
					rows = append(rows, [2]string{"Env", formatStringMap(server.Env)})
				}
				if len(server.Headers) > 0 {
					rows = append(rows, [2]string{"Headers", formatStringMap(server.Headers)})
				}
				if len(server.Tools) > 0 {
					rows = append(rows, [2]string{"Tools", strings.Join(server.Tools, ", ")})
				}
				if server.Error != "" {
					rows = append(rows, [2]string{"Error", server.Error})
				}
				rows = append(rows,
					[2]string{"CreatedAt", server.CreatedAt.Format("2006-01-02 15:04:05")},
					[2]string{"UpdatedAt", server.UpdatedAt.Format("2006-01-02 15:04:05")},
				)
				return writeKeyValues(cmd, rows)
			})
		},
	}
}

func newMCPAddCmd() *cobra.Command {
	var stdioCommand string
	var sseURL string
	var httpURL string
	var argValues []string
	var envValues []string
	var headerValues []string

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add an MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dto, err := buildMCPCreateDTO(args[0], stdioCommand, sseURL, httpURL, argValues, envValues, headerValues)
			if err != nil {
				return withExitCode(err, 2)
			}

			return runWithRuntime(cmd, true, false, func(runtime *Runtime) error {
				server, err := actions.CreateMCPServer(runtime.Backend, dto)
				if err != nil {
					return err
				}
				return writeActionResult(cmd, server, "MCP server added: %s", server.Name)
			})
		},
	}

	cmd.Flags().StringVar(&stdioCommand, "stdio", "", "stdio transport command")
	cmd.Flags().StringVar(&sseURL, "sse", "", "sse transport URL")
	cmd.Flags().StringVar(&httpURL, "http", "", "streamable-http transport URL")
	cmd.Flags().StringArrayVar(&argValues, "arg", nil, "stdio argument, can be repeated")
	cmd.Flags().StringArrayVar(&envValues, "env", nil, "stdio environment variable in KEY=VALUE form, can be repeated")
	cmd.Flags().StringArrayVar(&headerValues, "header", nil, "HTTP header in KEY=VALUE form, can be repeated")
	cmd.MarkFlagsMutuallyExclusive("stdio", "sse", "http")

	return cmd
}

func newMCPUpdateCmd() *cobra.Command {
	var newName string
	var enabledValue string
	var commandValue string
	var urlValue string
	var argValues []string
	var envValues []string
	var headerValues []string

	cmd := &cobra.Command{
		Use:   "update <name-or-id>",
		Short: "Update an MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, true, false, func(runtime *Runtime) error {
				current, err := actions.GetMCPServer(runtime.Backend, args[0])
				if err != nil {
					return err
				}

				dto, changed, err := buildMCPUpdateDTO(cmd, current, newName, enabledValue, commandValue, urlValue, argValues, envValues, headerValues)
				if err != nil {
					return withExitCode(err, 2)
				}
				if !changed {
					return withExitCode(fmt.Errorf("no changes specified"), 2)
				}

				server, err := actions.UpdateMCPServer(runtime.Backend, args[0], dto)
				if err != nil {
					return err
				}
				return writeActionResult(cmd, server, "MCP server updated: %s", server.Name)
			})
		},
	}

	cmd.Flags().StringVar(&newName, "name", "", "new server name")
	cmd.Flags().StringVar(&enabledValue, "enabled", "", "set enabled to true or false")
	cmd.Flags().StringVar(&commandValue, "command", "", "stdio command")
	cmd.Flags().StringVar(&urlValue, "url", "", "sse or streamable-http URL")
	cmd.Flags().StringArrayVar(&argValues, "arg", nil, "stdio argument, can be repeated")
	cmd.Flags().StringArrayVar(&envValues, "env", nil, "stdio environment variable in KEY=VALUE form, can be repeated")
	cmd.Flags().StringArrayVar(&headerValues, "header", nil, "HTTP header in KEY=VALUE form, can be repeated")

	return cmd
}

func newMCPRemoveCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "remove <name-or-id>",
		Short: "Remove an MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, true, false, func(runtime *Runtime) error {
				server, err := actions.GetMCPServer(runtime.Backend, args[0])
				if err != nil {
					return err
				}

				if !yes {
					if !isInteractiveTerminal() {
						return withExitCode(fmt.Errorf("use --yes when stdin/stdout is not a terminal"), 2)
					}
					confirmed, err := confirmAction(cmd, fmt.Sprintf("Delete MCP server '%s'? [y/N]: ", server.Name))
					if err != nil {
						return err
					}
					if !confirmed {
						return nil
					}
				}

				deleted, err := actions.DeleteMCPServer(runtime.Backend, args[0])
				if err != nil {
					return err
				}
				result := map[string]any{
					"id":      deleted.ID,
					"name":    deleted.Name,
					"deleted": true,
				}
				return writeActionResult(cmd, result, "MCP server removed: %s", deleted.Name)
			})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation")
	return cmd
}

func newMCPConnectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "connect <name-or-id>",
		Short: "Connect an MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, true, false, func(runtime *Runtime) error {
				server, err := actions.ConnectMCPServer(runtime.Backend, args[0])
				if err != nil {
					return err
				}
				return writeActionResult(cmd, server, "MCP server connected: %s", server.Name)
			})
		},
	}
}

func newMCPDisconnectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disconnect <name-or-id>",
		Short: "Disconnect an MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, true, false, func(runtime *Runtime) error {
				server, err := actions.DisconnectMCPServer(runtime.Backend, args[0])
				if err != nil {
					return err
				}
				return writeActionResult(cmd, server, "MCP server disconnected: %s", server.Name)
			})
		},
	}
}

func buildMCPCreateDTO(name string, stdioCommand string, sseURL string, httpURL string, argValues []string, envValues []string, headerValues []string) (*models.CreateMCPServerDto, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	transportCount := 0
	if strings.TrimSpace(stdioCommand) != "" {
		transportCount++
	}
	if strings.TrimSpace(sseURL) != "" {
		transportCount++
	}
	if strings.TrimSpace(httpURL) != "" {
		transportCount++
	}
	if transportCount != 1 {
		return nil, fmt.Errorf("exactly one of --stdio, --sse, or --http is required")
	}

	dto := &models.CreateMCPServerDto{Name: name}

	switch {
	case strings.TrimSpace(stdioCommand) != "":
		headers, err := parseKeyValuePairs(headerValues)
		if err != nil {
			return nil, err
		}
		if len(headers) > 0 {
			return nil, fmt.Errorf("--header can only be used with --sse or --http")
		}
		env, err := parseKeyValuePairs(envValues)
		if err != nil {
			return nil, err
		}
		dto.Transport = models.MCPTransportStdio
		dto.Command = strings.TrimSpace(stdioCommand)
		dto.Args = append([]string(nil), argValues...)
		dto.Env = env
	case strings.TrimSpace(sseURL) != "":
		env, err := parseKeyValuePairs(envValues)
		if err != nil {
			return nil, err
		}
		if len(env) > 0 || len(argValues) > 0 {
			return nil, fmt.Errorf("--arg and --env can only be used with --stdio")
		}
		headers, err := parseKeyValuePairs(headerValues)
		if err != nil {
			return nil, err
		}
		dto.Transport = models.MCPTransportSSE
		dto.URL = strings.TrimSpace(sseURL)
		dto.Headers = headers
	default:
		env, err := parseKeyValuePairs(envValues)
		if err != nil {
			return nil, err
		}
		if len(env) > 0 || len(argValues) > 0 {
			return nil, fmt.Errorf("--arg and --env can only be used with --stdio")
		}
		headers, err := parseKeyValuePairs(headerValues)
		if err != nil {
			return nil, err
		}
		dto.Transport = models.MCPTransportStreamableHTTP
		dto.URL = strings.TrimSpace(httpURL)
		dto.Headers = headers
	}

	return dto, nil
}

func buildMCPUpdateDTO(cmd *cobra.Command, current *models.MCPServerDto, newName string, enabledValue string, commandValue string, urlValue string, argValues []string, envValues []string, headerValues []string) (*models.UpdateMCPServerDto, bool, error) {
	dto := &models.UpdateMCPServerDto{}
	changed := false

	if cmd.Flags().Changed("name") {
		newName = strings.TrimSpace(newName)
		if newName == "" {
			return nil, false, fmt.Errorf("--name cannot be empty")
		}
		dto.Name = newName
		changed = true
	}

	if cmd.Flags().Changed("enabled") {
		enabled, err := strconv.ParseBool(enabledValue)
		if err != nil {
			return nil, false, fmt.Errorf("--enabled must be true or false")
		}
		dto.Enabled = &enabled
		changed = true
	}

	switch current.Transport {
	case models.MCPTransportStdio:
		if cmd.Flags().Changed("url") || cmd.Flags().Changed("header") {
			return nil, false, fmt.Errorf("--url and --header can only be used with sse or streamable-http servers")
		}
		if cmd.Flags().Changed("command") {
			commandValue = strings.TrimSpace(commandValue)
			if commandValue == "" {
				return nil, false, fmt.Errorf("--command cannot be empty")
			}
			dto.Command = commandValue
			changed = true
		}
		if cmd.Flags().Changed("arg") {
			dto.Args = append([]string(nil), argValues...)
			changed = true
		}
		if cmd.Flags().Changed("env") {
			env, err := parseKeyValuePairs(envValues)
			if err != nil {
				return nil, false, err
			}
			dto.Env = env
			changed = true
		}
	case models.MCPTransportSSE, models.MCPTransportStreamableHTTP:
		if cmd.Flags().Changed("command") || cmd.Flags().Changed("arg") || cmd.Flags().Changed("env") {
			return nil, false, fmt.Errorf("--command, --arg, and --env can only be used with stdio servers")
		}
		if cmd.Flags().Changed("url") {
			urlValue = strings.TrimSpace(urlValue)
			if urlValue == "" {
				return nil, false, fmt.Errorf("--url cannot be empty")
			}
			dto.URL = urlValue
			changed = true
		}
		if cmd.Flags().Changed("header") {
			headers, err := parseKeyValuePairs(headerValues)
			if err != nil {
				return nil, false, err
			}
			dto.Headers = headers
			changed = true
		}
	}

	return dto, changed, nil
}

func parseKeyValuePairs(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	result := make(map[string]string, len(values))
	for _, value := range values {
		key, item, ok := strings.Cut(value, "=")
		if !ok {
			return nil, fmt.Errorf("invalid key/value pair %q, expected KEY=VALUE", value)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("invalid key/value pair %q, key cannot be empty", value)
		}
		result[key] = item
	}
	return result, nil
}

func formatStringMap(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, values[key]))
	}
	return strings.Join(parts, ", ")
}

func emptyOrDefault(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func mcpTarget(server models.MCPServerDto) string {
	if server.Transport == models.MCPTransportStdio {
		if len(server.Args) == 0 {
			return server.Command
		}
		return strings.TrimSpace(server.Command + " " + strings.Join(server.Args, " "))
	}
	return server.URL
}

func maxPage(total int64, pageSize int) int64 {
	if total == 0 {
		return 1
	}
	return (total + int64(pageSize) - 1) / int64(pageSize)
}
