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
	"github.com/masteryyh/agenty/pkg/cli/actions"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/spf13/cobra"
)

func newGatewayCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "Manage gateway resources",
	}
	cmd.AddCommand(newGatewayChannelCmd())
	cmd.AddCommand(newGatewayBindingCmd())
	cmd.AddCommand(newGatewayBindCmd())
	return cmd
}

func newGatewayChannelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "channel",
		Short: "Manage gateway channels",
	}
	cmd.AddCommand(newGatewayChannelListCmd())
	cmd.AddCommand(newGatewayChannelGetCmd())
	cmd.AddCommand(newGatewayChannelAddCmd())
	cmd.AddCommand(newGatewayChannelUpdateCmd())
	cmd.AddCommand(newGatewayChannelRemoveCmd())
	return cmd
}

func newGatewayChannelListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List gateway channels",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if page <= 0 || pageSize <= 0 || pageSize > 100 {
				return withExitCode(fmt.Errorf("--page must be >= 1 and --page-size must be between 1 and 100"), 2)
			}
			return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
				result, err := actions.ListChannels(runtime.Backend, page, pageSize)
				if err != nil {
					return err
				}
				if outputJSON {
					return writeJSON(cmd, result)
				}
				if result == nil || len(result.Data) == 0 {
					return writeLine(cmd, "No gateway channels.")
				}
				rows := make([][]string, 0, len(result.Data))
				for _, ch := range result.Data {
					rows = append(rows, []string{
						ch.ID,
						string(ch.Type),
						ch.AccountID,
						strconv.FormatBool(ch.Enabled),
						strconv.FormatBool(ch.Required),
						strconv.FormatBool(ch.SendReasoning),
						strconv.FormatBool(ch.SendToolEvents),
					})
				}
				if err := writeTable(cmd, []string{"ID", "Type", "AccountID", "Enabled", "Required", "Reasoning", "ToolEvents"}, rows); err != nil {
					return err
				}
				return writeLine(cmd, "Page %d/%d  Total %d", result.Page, maxPage(result.Total, result.PageSize), result.Total)
			})
		},
	}
}

func newGatewayChannelGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <channel-id>",
		Short: "Show gateway channel details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
				channel, err := actions.GetChannel(runtime.Backend, args[0])
				if err != nil {
					return err
				}
				if outputJSON {
					return writeJSON(cmd, channel)
				}
				rows := [][2]string{
					{"ID", channel.ID},
					{"Type", string(channel.Type)},
					{"AccountID", channel.AccountID},
					{"Enabled", strconv.FormatBool(channel.Enabled)},
					{"Required", strconv.FormatBool(channel.Required)},
					{"SendReasoning", strconv.FormatBool(channel.SendReasoning)},
					{"SendToolEvents", strconv.FormatBool(channel.SendToolEvents)},
					{"RequireMention", strconv.FormatBool(channel.RequireMention)},
				}
				if channel.Discord != nil {
					rows = append(rows,
						[2]string{"DiscordDMEnabled", strconv.FormatBool(channel.Discord.DMEnabled)},
						[2]string{"DiscordGuildAllowlist", strings.Join(channel.Discord.GuildAllowlist, ", ")},
						[2]string{"DiscordAllowedChannelIDs", strings.Join(channel.Discord.AllowedChannelIDs, ", ")},
						[2]string{"DiscordAllowedUserIDs", strings.Join(channel.Discord.AllowedUserIDs, ", ")},
					)
				}
				return writeKeyValues(cmd, rows)
			})
		},
	}
}

func newGatewayChannelAddCmd() *cobra.Command {
	var channelType string
	var accountID string
	var enabled bool
	var required bool
	var sendReasoning bool
	var sendToolEvents bool
	var requireMention bool
	var botToken string
	var appID string
	var publicKey string
	var guildAllowlist []string
	var allowedChannelIDs []string
	var allowedUserIDs []string
	var dmEnabled bool

	cmd := &cobra.Command{
		Use:   "add <channel-id>",
		Short: "Add a gateway channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dto := &models.CreateGatewayChannelDto{
				ID:             strings.TrimSpace(args[0]),
				Type:           strings.TrimSpace(channelType),
				AccountID:      strings.TrimSpace(accountID),
				Enabled:        &enabled,
				Required:       required,
				SendReasoning:  sendReasoning,
				SendToolEvents: sendToolEvents,
				RequireMention: requireMention,
			}
			if dto.Type == string(models.ChannelTypeDiscord) {
				dto.Discord = &models.GatewayDiscordChannelConfig{
					BotToken:          botToken,
					AppID:             appID,
					PublicKey:         publicKey,
					GuildAllowlist:    guildAllowlist,
					AllowedChannelIDs: allowedChannelIDs,
					AllowedUserIDs:    allowedUserIDs,
					DMEnabled:         dmEnabled,
				}
			}
			return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
				channel, err := actions.CreateChannel(runtime.Backend, dto)
				if err != nil {
					return err
				}
				return writeActionResult(cmd, channel, "Gateway channel added: %s", channel.ID)
			})
		},
	}

	cmd.Flags().StringVar(&channelType, "type", "", "channel type, currently discord")
	cmd.Flags().StringVar(&accountID, "account-id", "", "logical bot/account id")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "enable this channel")
	cmd.Flags().BoolVar(&required, "required", false, "fail server startup when this channel cannot start")
	cmd.Flags().BoolVar(&sendReasoning, "send-reasoning", false, "send reasoning events to the channel")
	cmd.Flags().BoolVar(&sendToolEvents, "send-tool-events", false, "send tool status events to the channel")
	cmd.Flags().BoolVar(&requireMention, "require-mention", false, "only react when the bot is mentioned")
	cmd.Flags().StringVar(&botToken, "bot-token", "", "discord bot token")
	cmd.Flags().StringVar(&appID, "app-id", "", "discord app id")
	cmd.Flags().StringVar(&publicKey, "public-key", "", "discord public key")
	cmd.Flags().StringArrayVar(&guildAllowlist, "guild", nil, "discord guild id allowlist, can be repeated")
	cmd.Flags().StringArrayVar(&allowedChannelIDs, "channel", nil, "discord channel id allowlist, can be repeated")
	cmd.Flags().StringArrayVar(&allowedUserIDs, "user", nil, "discord user id allowlist, can be repeated")
	cmd.Flags().BoolVar(&dmEnabled, "dm-enabled", false, "accept Discord direct messages")
	cmd.MarkFlagRequired("type")
	cmd.MarkFlagRequired("account-id")
	return cmd
}

func newGatewayChannelUpdateCmd() *cobra.Command {
	var channelType string
	var accountID string
	var enabledValue string
	var requiredValue string
	var sendReasoningValue string
	var sendToolEventsValue string
	var requireMentionValue string
	var botToken string
	var appID string
	var publicKey string
	var guildAllowlist []string
	var allowedChannelIDs []string
	var allowedUserIDs []string
	var dmEnabledValue string

	cmd := &cobra.Command{
		Use:   "update <channel-id>",
		Short: "Update a gateway channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
				current, err := actions.GetChannel(runtime.Backend, args[0])
				if err != nil {
					return err
				}
				dto, changed, err := buildGatewayChannelUpdateDTO(current, channelType, accountID, enabledValue, requiredValue, sendReasoningValue, sendToolEventsValue, requireMentionValue, botToken, appID, publicKey, guildAllowlist, allowedChannelIDs, allowedUserIDs, dmEnabledValue)
				if err != nil {
					return withExitCode(err, 2)
				}
				if !changed {
					return withExitCode(fmt.Errorf("no changes specified"), 2)
				}
				channel, err := actions.UpdateChannel(runtime.Backend, args[0], dto)
				if err != nil {
					return err
				}
				return writeActionResult(cmd, channel, "Gateway channel updated: %s", channel.ID)
			})
		},
	}

	cmd.Flags().StringVar(&channelType, "type", "", "channel type")
	cmd.Flags().StringVar(&accountID, "account-id", "", "logical bot/account id")
	cmd.Flags().StringVar(&enabledValue, "enabled", "", "set enabled to true or false")
	cmd.Flags().StringVar(&requiredValue, "required", "", "set required to true or false")
	cmd.Flags().StringVar(&sendReasoningValue, "send-reasoning", "", "set sendReasoning to true or false")
	cmd.Flags().StringVar(&sendToolEventsValue, "send-tool-events", "", "set sendToolEvents to true or false")
	cmd.Flags().StringVar(&requireMentionValue, "require-mention", "", "set requireMention to true or false")
	cmd.Flags().StringVar(&botToken, "bot-token", "", "discord bot token")
	cmd.Flags().StringVar(&appID, "app-id", "", "discord app id")
	cmd.Flags().StringVar(&publicKey, "public-key", "", "discord public key")
	cmd.Flags().StringArrayVar(&guildAllowlist, "guild", nil, "discord guild id allowlist, can be repeated")
	cmd.Flags().StringArrayVar(&allowedChannelIDs, "channel", nil, "discord channel id allowlist, can be repeated")
	cmd.Flags().StringArrayVar(&allowedUserIDs, "user", nil, "discord user id allowlist, can be repeated")
	cmd.Flags().StringVar(&dmEnabledValue, "dm-enabled", "", "set Discord DM mode to true or false")
	return cmd
}

func newGatewayChannelRemoveCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <channel-id>",
		Short: "Remove a gateway channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
				channel, err := actions.GetChannel(runtime.Backend, args[0])
				if err != nil {
					return err
				}
				if !yes {
					if !isInteractiveTerminal() {
						return withExitCode(fmt.Errorf("use --yes when stdin/stdout is not a terminal"), 2)
					}
					confirmed, err := confirmAction(cmd, fmt.Sprintf("Delete gateway channel '%s'? [y/N]: ", channel.ID))
					if err != nil {
						return err
					}
					if !confirmed {
						return nil
					}
				}
				deleted, err := actions.DeleteChannel(runtime.Backend, args[0])
				if err != nil {
					return err
				}
				return writeActionResult(cmd, map[string]any{"id": deleted.ID, "deleted": true}, "Gateway channel removed: %s", deleted.ID)
			})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation")
	return cmd
}

func newGatewayBindingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "binding",
		Short: "Manage gateway account bindings",
	}
	cmd.AddCommand(newGatewayBindingListCmd())
	cmd.AddCommand(newGatewayBindingAddCmd())
	cmd.AddCommand(newGatewayBindingUpdateCmd())
	cmd.AddCommand(newGatewayBindingRemoveCmd())
	return cmd
}

func newGatewayBindingListCmd() *cobra.Command {
	var agentIDRaw string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List gateway account bindings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var agentID *uuid.UUID
			if strings.TrimSpace(agentIDRaw) != "" {
				parsed, err := uuid.Parse(strings.TrimSpace(agentIDRaw))
				if err != nil {
					return withExitCode(fmt.Errorf("invalid --agent-id value"), 2)
				}
				agentID = &parsed
			}
			return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
				bindings, err := actions.ListGatewayBindings(runtime.Backend, agentID)
				if err != nil {
					return err
				}
				if outputJSON {
					return writeJSON(cmd, bindings)
				}
				if len(bindings) == 0 {
					return writeLine(cmd, "No gateway account bindings.")
				}
				rows := make([][]string, 0, len(bindings))
				for _, binding := range bindings {
					defaultModelID := ""
					if binding.DefaultModelID != nil {
						defaultModelID = binding.DefaultModelID.String()
					}
					rows = append(rows, []string{
						binding.ID.String(),
						binding.AgentID.String(),
						binding.ChannelID,
						binding.ChannelType,
						binding.AccountID,
						strconv.FormatBool(binding.Enabled),
						defaultModelID,
					})
				}
				return writeTable(cmd, []string{"ID", "AgentID", "ChannelID", "ChannelType", "AccountID", "Enabled", "DefaultModelID"}, rows)
			})
		},
	}
	cmd.Flags().StringVar(&agentIDRaw, "agent-id", "", "filter bindings by agent id")
	return cmd
}

func newGatewayBindingAddCmd() *cobra.Command {
	var defaultModelIDRaw string
	var enabled bool
	cmd := &cobra.Command{
		Use:   "add <agent-id> <channel-id>",
		Short: "Bind an agent to a gateway channel account",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreateGatewayBinding(cmd, args[0], args[1], defaultModelIDRaw, enabled)
		},
	}
	cmd.Flags().StringVar(&defaultModelIDRaw, "model-id", "", "default model id for this binding")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "enable this binding")
	return cmd
}

func newGatewayBindCmd() *cobra.Command {
	var defaultModelIDRaw string
	var enabled bool
	cmd := &cobra.Command{
		Use:   "bind <agent-id> <channel-id>",
		Short: "Bind an agent to a gateway channel account",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreateGatewayBinding(cmd, args[0], args[1], defaultModelIDRaw, enabled)
		},
	}
	cmd.Flags().StringVar(&defaultModelIDRaw, "model-id", "", "default model id for this binding")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "enable this binding")
	return cmd
}

func newGatewayBindingUpdateCmd() *cobra.Command {
	var defaultModelIDRaw string
	var clearDefaultModel bool
	var enabledValue string
	cmd := &cobra.Command{
		Use:   "update <agent-id> <binding-id>",
		Short: "Update a gateway account binding",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentID, err := uuid.Parse(strings.TrimSpace(args[0]))
			if err != nil {
				return withExitCode(fmt.Errorf("invalid agent id"), 2)
			}
			bindingID, err := uuid.Parse(strings.TrimSpace(args[1]))
			if err != nil {
				return withExitCode(fmt.Errorf("invalid binding id"), 2)
			}
			dto := &models.UpdateAgentGatewayBindingDto{}
			changed := false
			if strings.TrimSpace(defaultModelIDRaw) != "" {
				if clearDefaultModel {
					return withExitCode(fmt.Errorf("--model-id and --clear-model cannot be used together"), 2)
				}
				defaultModelID, err := uuid.Parse(strings.TrimSpace(defaultModelIDRaw))
				if err != nil {
					return withExitCode(fmt.Errorf("invalid --model-id value"), 2)
				}
				dto.DefaultModelID = &defaultModelID
				dto.DefaultModelIDSet = true
				changed = true
			}
			if clearDefaultModel {
				dto.DefaultModelIDSet = true
				changed = true
			}
			if enabledValue != "" {
				enabled, err := strconv.ParseBool(enabledValue)
				if err != nil {
					return withExitCode(fmt.Errorf("invalid --enabled value"), 2)
				}
				dto.Enabled = &enabled
				changed = true
			}
			if !changed {
				return withExitCode(fmt.Errorf("no changes specified"), 2)
			}
			return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
				binding, err := actions.UpdateGatewayBinding(runtime.Backend, agentID, bindingID, dto)
				if err != nil {
					return err
				}
				return writeActionResult(cmd, binding, "Gateway account binding updated: %s", binding.ID)
			})
		},
	}
	cmd.Flags().StringVar(&defaultModelIDRaw, "model-id", "", "default model id for this binding")
	cmd.Flags().BoolVar(&clearDefaultModel, "clear-model", false, "clear the default model override")
	cmd.Flags().StringVar(&enabledValue, "enabled", "", "set enabled to true or false")
	return cmd
}

func newGatewayBindingRemoveCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <agent-id> <binding-id>",
		Short: "Remove a gateway account binding",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentID, err := uuid.Parse(strings.TrimSpace(args[0]))
			if err != nil {
				return withExitCode(fmt.Errorf("invalid agent id"), 2)
			}
			bindingID, err := uuid.Parse(strings.TrimSpace(args[1]))
			if err != nil {
				return withExitCode(fmt.Errorf("invalid binding id"), 2)
			}
			if !yes {
				if !isInteractiveTerminal() {
					return withExitCode(fmt.Errorf("use --yes when stdin/stdout is not a terminal"), 2)
				}
				confirmed, err := confirmAction(cmd, fmt.Sprintf("Delete gateway account binding '%s'? [y/N]: ", bindingID))
				if err != nil {
					return err
				}
				if !confirmed {
					return nil
				}
			}
			return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
				if err := actions.DeleteGatewayBinding(runtime.Backend, agentID, bindingID); err != nil {
					return err
				}
				return writeActionResult(cmd, map[string]any{"id": bindingID, "deleted": true}, "Gateway account binding removed: %s", bindingID)
			})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation")
	return cmd
}

func runCreateGatewayBinding(cmd *cobra.Command, agentIDRaw, channelIDRaw, defaultModelIDRaw string, enabled bool) error {
	agentID, err := uuid.Parse(strings.TrimSpace(agentIDRaw))
	if err != nil {
		return withExitCode(fmt.Errorf("invalid agent id"), 2)
	}
	var defaultModelID *uuid.UUID
	if strings.TrimSpace(defaultModelIDRaw) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(defaultModelIDRaw))
		if err != nil {
			return withExitCode(fmt.Errorf("invalid --model-id value"), 2)
		}
		defaultModelID = &parsed
	}
	dto := &models.CreateAgentGatewayBindingDto{
		ChannelID:      strings.TrimSpace(channelIDRaw),
		DefaultModelID: defaultModelID,
		Enabled:        &enabled,
	}
	return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
		binding, err := actions.CreateGatewayBinding(runtime.Backend, agentID, dto)
		if err != nil {
			return err
		}
		return writeActionResult(cmd, binding, "Gateway account binding added: %s", binding.ID)
	})
}

func buildGatewayChannelUpdateDTO(current *models.GatewayChannelDto, channelType, accountID, enabledValue, requiredValue, sendReasoningValue, sendToolEventsValue, requireMentionValue, botToken, appID, publicKey string, guildAllowlist, allowedChannelIDs, allowedUserIDs []string, dmEnabledValue string) (*models.UpdateGatewayChannelDto, bool, error) {
	dto := &models.UpdateGatewayChannelDto{}
	changed := false

	if t := strings.TrimSpace(channelType); t != "" && t != string(current.Type) {
		dto.Type = t
		changed = true
	}
	if a := strings.TrimSpace(accountID); a != "" && a != current.AccountID {
		dto.AccountID = a
		changed = true
	}
	if enabledValue != "" {
		v, err := strconv.ParseBool(enabledValue)
		if err != nil {
			return nil, false, fmt.Errorf("invalid --enabled value")
		}
		dto.Enabled = &v
		changed = true
	}
	if requiredValue != "" {
		v, err := strconv.ParseBool(requiredValue)
		if err != nil {
			return nil, false, fmt.Errorf("invalid --required value")
		}
		dto.Required = &v
		changed = true
	}
	if sendReasoningValue != "" {
		v, err := strconv.ParseBool(sendReasoningValue)
		if err != nil {
			return nil, false, fmt.Errorf("invalid --send-reasoning value")
		}
		dto.SendReasoning = &v
		changed = true
	}
	if sendToolEventsValue != "" {
		v, err := strconv.ParseBool(sendToolEventsValue)
		if err != nil {
			return nil, false, fmt.Errorf("invalid --send-tool-events value")
		}
		dto.SendToolEvents = &v
		changed = true
	}
	if requireMentionValue != "" {
		v, err := strconv.ParseBool(requireMentionValue)
		if err != nil {
			return nil, false, fmt.Errorf("invalid --require-mention value")
		}
		dto.RequireMention = &v
		changed = true
	}

	if current.Type == models.ChannelTypeDiscord || strings.TrimSpace(channelType) == string(models.ChannelTypeDiscord) {
		discord := &models.GatewayDiscordChannelConfig{}
		if current.Discord != nil {
			*discord = *current.Discord
		}
		discordChanged := false
		if botToken != "" {
			discord.BotToken = botToken
			discordChanged = true
		}
		if appID != "" {
			discord.AppID = appID
			discordChanged = true
		}
		if publicKey != "" {
			discord.PublicKey = publicKey
			discordChanged = true
		}
		if guildAllowlist != nil {
			discord.GuildAllowlist = guildAllowlist
			discordChanged = true
		}
		if allowedChannelIDs != nil {
			discord.AllowedChannelIDs = allowedChannelIDs
			discordChanged = true
		}
		if allowedUserIDs != nil {
			discord.AllowedUserIDs = allowedUserIDs
			discordChanged = true
		}
		if dmEnabledValue != "" {
			v, err := strconv.ParseBool(dmEnabledValue)
			if err != nil {
				return nil, false, fmt.Errorf("invalid --dm-enabled value")
			}
			discord.DMEnabled = v
			discordChanged = true
		}
		if discordChanged {
			dto.Discord = discord
			changed = true
		}
	}

	return dto, changed, nil
}
