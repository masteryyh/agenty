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
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/spf13/cobra"
)

func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "agent", Short: "Inspect agents"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List agents",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
				result, err := runtime.Backend.ListAgents(page, pageSize)
				if err != nil {
					return err
				}
				if outputJSON {
					return writeJSON(cmd, result)
				}
				rows := make([][]string, 0, len(result.Data))
				for _, agent := range result.Data {
					modelsText := make([]string, 0, len(agent.Models))
					for _, model := range agent.Models {
						modelsText = append(modelsText, modelDisplayName(model))
					}
					rows = append(rows, []string{
						agent.ID.String(),
						agent.Name,
						strconv.FormatBool(agent.IsDefault),
						strings.Join(modelsText, ", "),
					})
				}
				if len(rows) == 0 {
					return writeLine(cmd, "No agents.")
				}
				return writeTable(cmd, []string{"ID", "Name", "Default", "Models"}, rows)
			})
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "get <name-or-id>",
		Short: "Show agent details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
				agent, err := resolveAgentReference(runtime.Backend, args[0])
				if err != nil {
					return err
				}
				if outputJSON {
					return writeJSON(cmd, agent)
				}
				modelNames := make([]string, 0, len(agent.Models))
				for _, model := range agent.Models {
					modelNames = append(modelNames, modelDisplayName(model))
				}
				return writeKeyValues(cmd, [][2]string{
					{"ID", agent.ID.String()},
					{"Name", agent.Name},
					{"Default", strconv.FormatBool(agent.IsDefault)},
					{"Models", strings.Join(modelNames, ", ")},
					{"CreatedAt", agent.CreatedAt.Format("2006-01-02 15:04:05")},
					{"UpdatedAt", agent.UpdatedAt.Format("2006-01-02 15:04:05")},
				})
			})
		},
	})
	return cmd
}

func newProviderCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "provider", Short: "Inspect providers"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List providers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
				result, err := runtime.Backend.ListProviders(page, pageSize)
				if err != nil {
					return err
				}
				if outputJSON {
					return writeJSON(cmd, result)
				}
				rows := make([][]string, 0, len(result.Data))
				for _, provider := range result.Data {
					rows = append(rows, []string{
						provider.ID.String(),
						provider.Name,
						string(provider.Type),
						provider.BaseURL,
						provider.APIKeyCensored,
					})
				}
				if len(rows) == 0 {
					return writeLine(cmd, "No providers.")
				}
				return writeTable(cmd, []string{"ID", "Name", "Type", "Base URL", "API Key"}, rows)
			})
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "get <name-or-id>",
		Short: "Show provider details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
				provider, err := resolveProviderReference(runtime.Backend, args[0])
				if err != nil {
					return err
				}
				if outputJSON {
					return writeJSON(cmd, provider)
				}
				return writeKeyValues(cmd, [][2]string{
					{"ID", provider.ID.String()},
					{"Name", provider.Name},
					{"Type", string(provider.Type)},
					{"Base URL", provider.BaseURL},
					{"API Key", provider.APIKeyCensored},
					{"Preset", strconv.FormatBool(provider.IsPreset)},
					{"CreatedAt", provider.CreatedAt.Format("2006-01-02 15:04:05")},
					{"UpdatedAt", provider.UpdatedAt.Format("2006-01-02 15:04:05")},
				})
			})
		},
	})
	return cmd
}

func newModelCmd() *cobra.Command {
	var chatOnly bool
	var embeddingOnly bool

	cmd := &cobra.Command{Use: "model", Short: "Inspect models"}
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List models",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if chatOnly && embeddingOnly {
				return withExitCode(fmt.Errorf("--chat-only and --embedding-only cannot be used together"), 2)
			}
			return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
				result, err := runtime.Backend.ListModels(page, pageSize)
				if err != nil {
					return err
				}
				filtered := make([]models.ModelDto, 0, len(result.Data))
				for _, model := range result.Data {
					if chatOnly && model.EmbeddingModel {
						continue
					}
					if embeddingOnly && !model.EmbeddingModel {
						continue
					}
					filtered = append(filtered, model)
				}
				if outputJSON {
					out := &pagination.PagedResponse[models.ModelDto]{
						Total:    int64(len(filtered)),
						Page:     result.Page,
						PageSize: result.PageSize,
						Data:     filtered,
					}
					return writeJSON(cmd, out)
				}
				rows := make([][]string, 0, len(filtered))
				for _, model := range filtered {
					kind := "chat"
					if model.EmbeddingModel {
						kind = "embedding"
					}
					rows = append(rows, []string{
						model.ID.String(),
						modelDisplayName(model),
						model.Code,
						kind,
						strconv.FormatBool(model.DefaultModel),
						strconv.FormatBool(modelProviderConfigured(model)),
					})
				}
				if len(rows) == 0 {
					return writeLine(cmd, "No models.")
				}
				return writeTable(cmd, []string{"ID", "Model", "Code", "Kind", "Default", "Configured"}, rows)
			})
		},
	}
	listCmd.Flags().BoolVar(&chatOnly, "chat-only", false, "Show chat models only")
	listCmd.Flags().BoolVar(&embeddingOnly, "embedding-only", false, "Show embedding models only")
	cmd.AddCommand(listCmd)
	return cmd
}

func newSettingsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "settings", Short: "Inspect system settings"}
	cmd.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "Show system settings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
				settings, err := runtime.Backend.GetSystemSettings()
				if err != nil {
					return err
				}
				if outputJSON {
					return writeJSON(cmd, settings)
				}
				configured := make([]string, 0, len(settings.ConfiguredWebSearchProviders))
				for _, provider := range settings.ConfiguredWebSearchProviders {
					configured = append(configured, string(provider))
				}
				rows := [][2]string{
					{"Initialized", strconv.FormatBool(settings.Initialized)},
					{"Embedding model ID", formatUUIDPointer(settings.EmbeddingModelID)},
					{"Context compression model ID", formatUUIDPointer(settings.ContextCompressionModelID)},
					{"Web search provider", string(settings.WebSearchProvider)},
					{"Configured web search providers", strings.Join(configured, ", ")},
					{"Last configured web search provider", string(settings.LastConfiguredWebSearchProvider)},
					{"Brave API key", settings.BraveAPIKey},
					{"Tavily API key", settings.TavilyAPIKey},
					{"Firecrawl API key", settings.FirecrawlAPIKey},
					{"Firecrawl base URL", settings.FirecrawlBaseURL},
				}
				return writeKeyValues(cmd, rows)
			})
		},
	})
	return cmd
}

func newSessionCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "session", Short: "Inspect sessions"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
				result, err := runtime.Backend.ListSessions(page, pageSize)
				if err != nil {
					return err
				}
				if outputJSON {
					return writeJSON(cmd, result)
				}
				agents, _ := listAgentsAll(runtime.Backend)
				agentNames := make(map[uuid.UUID]string, len(agents))
				for _, agent := range agents {
					agentNames[agent.ID] = agent.Name
				}
				rows := make([][]string, 0, len(result.Data))
				for _, session := range result.Data {
					rows = append(rows, []string{
						session.ID.String(),
						agentNames[session.AgentID],
						strconv.FormatInt(session.TokenConsumed, 10),
						strconv.FormatInt(session.ContextTokens, 10),
						formatStringPointer(session.Cwd),
						session.UpdatedAt.Format("2006-01-02 15:04:05"),
					})
				}
				if len(rows) == 0 {
					return writeLine(cmd, "No sessions.")
				}
				return writeTable(cmd, []string{"ID", "Agent", "Tokens", "Context", "Cwd", "UpdatedAt"}, rows)
			})
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "get <id>",
		Short: "Show session details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID, err := uuid.Parse(strings.TrimSpace(args[0]))
			if err != nil {
				return withExitCode(fmt.Errorf("invalid session ID: %s", args[0]), 2)
			}
			return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
				session, err := runtime.Backend.GetSession(sessionID)
				if err != nil {
					return err
				}
				if outputJSON {
					return writeJSON(cmd, session)
				}
				rows := [][2]string{
					{"ID", session.ID.String()},
					{"Agent ID", session.AgentID.String()},
					{"Token consumed", strconv.FormatInt(session.TokenConsumed, 10)},
					{"Context tokens", strconv.FormatInt(session.ContextTokens, 10)},
					{"Last used model", session.LastUsedModel.String()},
					{"Last used thinking level", formatStringPointer(session.LastUsedThinkingLevel)},
					{"Active compaction ID", formatUUIDPointer(session.ActiveCompactionID)},
					{"Cwd", formatStringPointer(session.Cwd)},
					{"Messages", strconv.Itoa(len(session.Messages))},
					{"CreatedAt", session.CreatedAt.Format("2006-01-02 15:04:05")},
					{"UpdatedAt", session.UpdatedAt.Format("2006-01-02 15:04:05")},
				}
				return writeKeyValues(cmd, rows)
			})
		},
	})
	return cmd
}

func newMemoryCmd() *cobra.Command {
	var agentRef string

	cmd := &cobra.Command{Use: "memory", Short: "Inspect agent memories"}
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List agent memories",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(agentRef) == "" {
				return withExitCode(fmt.Errorf("--agent is required"), 2)
			}
			return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
				agent, err := resolveAgentReference(runtime.Backend, agentRef)
				if err != nil {
					return err
				}
				items, err := runtime.Backend.ListMemories(agent.ID)
				if err != nil {
					return err
				}
				if outputJSON {
					return writeJSON(cmd, items)
				}
				rows := make([][]string, 0, len(items))
				for _, item := range items {
					rows = append(rows, []string{
						item.ID.String(),
						item.Title,
						item.Preview,
						item.CreatedAt.Format("2006-01-02 15:04:05"),
					})
				}
				if len(rows) == 0 {
					return writeLine(cmd, "No memories.")
				}
				return writeTable(cmd, []string{"ID", "Title", "Preview", "CreatedAt"}, rows)
			})
		},
	}
	listCmd.Flags().StringVar(&agentRef, "agent", "", "Agent name or ID")
	cmd.AddCommand(listCmd)
	return cmd
}

func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "skill", Short: "Manage global skills"}
	cmd.AddCommand(&cobra.Command{
		Use:   "rescan",
		Short: "Rescan the agenty global skill index",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, false, true, func(runtime *Runtime) error {
				if err := runtime.Backend.RescanGlobalSkills(); err != nil {
					return err
				}
				result := map[string]any{"rescanned": true}
				return writeActionResult(cmd, result, "Global skills rescanned.")
			})
		},
	})
	return cmd
}

func resolveAgentReference(b backend.Backend, ref string) (*models.AgentDto, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, withExitCode(fmt.Errorf("agent reference is required"), 2)
	}

	agents, err := listAgentsAll(b)
	if err != nil {
		return nil, err
	}
	if id, err := uuid.Parse(ref); err == nil {
		for _, agent := range agents {
			if agent.ID == id {
				return &agent, nil
			}
		}
		return nil, withExitCode(fmt.Errorf("agent not found: %s", ref), 2)
	}

	matches := make([]models.AgentDto, 0, 1)
	for _, agent := range agents {
		if agent.Name == ref {
			matches = append(matches, agent)
		}
	}
	switch len(matches) {
	case 0:
		return nil, withExitCode(fmt.Errorf("agent not found: %s", ref), 2)
	case 1:
		return &matches[0], nil
	default:
		return nil, withExitCode(fmt.Errorf("agent name is ambiguous: %s; use agent ID instead", ref), 2)
	}
}

func listAgentsAll(b backend.Backend) ([]models.AgentDto, error) {
	return listAllPages(func(pageNum, pageSize int) (*pagination.PagedResponse[models.AgentDto], error) {
		return b.ListAgents(pageNum, pageSize)
	})
}

func formatUUIDPointer(value *uuid.UUID) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func formatStringPointer(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
