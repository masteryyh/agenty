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
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/models"
)

func handleGatewayCmd(b backend.Backend, bridge Bridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	for {
		result, err := b.ListChannels(1, 100)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to list gateway channels: %w", err)
		}

		if len(result.Data) == 0 {
			res, err := bridge.ShowList("Gateway Channels", []string{"(no channels)"}, "a add  ·  Esc back")
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if res.Action == ListActionAdd {
				if err := doCreateGatewayChannel(b, bridge); err != nil && !errors.Is(err, ErrCancelled) {
					bridge.Error("Failed to create gateway channel: %v", err)
				}
				continue
			}
			return CommandResult{Handled: true}, nil
		}

		items := make([]string, len(result.Data))
		for i, ch := range result.Data {
			status := "enabled"
			if !ch.Enabled {
				status = "disabled"
			}
			items[i] = fmt.Sprintf("%s (%s → %s)  %s", ch.ID, ch.Type, ch.AccountID, styleGray.Render(status))
		}

		res, err := bridge.ShowListWithCursorAndActions("Gateway Channels", items, ListHints, 0, nil, gatewayDeleteConfirm(result.Data))
		if err != nil {
			return CommandResult{Handled: true}, err
		}

		switch res.Action {
		case ListActionAdd:
			if err := doCreateGatewayChannel(b, bridge); err != nil && !errors.Is(err, ErrCancelled) {
				bridge.Error("Failed to create gateway channel: %v", err)
			}
		case ListActionEdit, ListActionSelect:
			if err := doUpdateGatewayChannel(b, bridge, result.Data[res.Index]); err != nil && !errors.Is(err, ErrCancelled) {
				bridge.Error("Failed to update gateway channel: %v", err)
			}
		case ListActionDelete:
			target := result.Data[res.Index]
			if err := b.DeleteChannel(target.ID); err != nil {
				bridge.Error("Failed to delete gateway channel: %v", err)
			} else {
				bridge.Success("Gateway channel deleted: %s", target.ID)
			}
		case ListActionCancel:
			return CommandResult{Handled: true}, nil
		}
	}
}

func gatewayDeleteConfirm(channels []models.GatewayChannelDto) func(idx int) string {
	return func(idx int) string {
		if idx < 0 || idx >= len(channels) {
			return ""
		}
		return fmt.Sprintf("Delete gateway channel '%s'?", channels[idx].ID)
	}
}

func doCreateGatewayChannel(b backend.Backend, bridge Bridge) error {
	var channelID string
	var accountID string
	var enabled bool = true
	var required bool
	var sendReasoning bool
	var sendToolEvents bool
	var requireMention bool
	var botToken string
	var guildAllowlist string
	var allowedChannelIDs string
	var allowedUserIDs string
	var dmEnabled bool

	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Channel ID").Value(&channelID),
		huh.NewInput().Title("Account ID").Value(&accountID),
		huh.NewSelect[bool]().Title("Enabled").Options(huh.NewOption("Yes", true), huh.NewOption("No", false)).Value(&enabled),
		huh.NewSelect[bool]().Title("Required").Options(huh.NewOption("No", false), huh.NewOption("Yes", true)).Value(&required),
		huh.NewSelect[bool]().Title("Send reasoning").Options(huh.NewOption("No", false), huh.NewOption("Yes", true)).Value(&sendReasoning),
		huh.NewSelect[bool]().Title("Send tool events").Options(huh.NewOption("No", false), huh.NewOption("Yes", true)).Value(&sendToolEvents),
		huh.NewSelect[bool]().Title("Require mention").Options(huh.NewOption("No", false), huh.NewOption("Yes", true)).Value(&requireMention),
		huh.NewInput().Title("Discord bot token").Value(&botToken),
		huh.NewInput().Title("Guild allowlist").Placeholder("comma-separated guild ids").Value(&guildAllowlist),
		huh.NewInput().Title("Allowed channel IDs").Placeholder("comma-separated channel ids").Value(&allowedChannelIDs),
		huh.NewInput().Title("Allowed user IDs").Placeholder("comma-separated user ids").Value(&allowedUserIDs),
		huh.NewSelect[bool]().Title("Allow DMs").Options(huh.NewOption("No", false), huh.NewOption("Yes", true)).Value(&dmEnabled),
	))

	submitted, err := bridge.ShowValidatedHuhForm(form, func() error {
		if strings.TrimSpace(channelID) == "" {
			return fmt.Errorf("Channel ID is required.")
		}
		if strings.TrimSpace(accountID) == "" {
			return fmt.Errorf("Account ID is required.")
		}
		if enabled && strings.TrimSpace(botToken) == "" {
			return fmt.Errorf("Discord bot token is required for enabled channels.")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !submitted {
		return ErrCancelled
	}

	channel, err := b.CreateChannel(&models.CreateGatewayChannelDto{
		ID:             strings.TrimSpace(channelID),
		Type:           string(models.ChannelTypeDiscord),
		AccountID:      strings.TrimSpace(accountID),
		Enabled:        &enabled,
		Required:       required,
		SendReasoning:  sendReasoning,
		SendToolEvents: sendToolEvents,
		RequireMention: requireMention,
		Discord: &models.GatewayDiscordChannelConfig{
			BotToken:          strings.TrimSpace(botToken),
			GuildAllowlist:    splitCSV(guildAllowlist),
			AllowedChannelIDs: splitCSV(allowedChannelIDs),
			AllowedUserIDs:    splitCSV(allowedUserIDs),
			DMEnabled:         dmEnabled,
		},
	})
	if err != nil {
		return err
	}
	bridge.Success("Gateway channel created: %s", channel.ID)
	return nil
}

func doUpdateGatewayChannel(b backend.Backend, bridge Bridge, target models.GatewayChannelDto) error {
	accountID := target.AccountID
	enabled := target.Enabled
	required := target.Required
	sendReasoning := target.SendReasoning
	sendToolEvents := target.SendToolEvents
	requireMention := target.RequireMention
	botToken := ""
	guildAllowlist := ""
	allowedChannelIDs := ""
	allowedUserIDs := ""
	dmEnabled := false

	if target.Discord != nil {
		guildAllowlist = strings.Join(target.Discord.GuildAllowlist, ",")
		allowedChannelIDs = strings.Join(target.Discord.AllowedChannelIDs, ",")
		allowedUserIDs = strings.Join(target.Discord.AllowedUserIDs, ",")
		dmEnabled = target.Discord.DMEnabled
	}

	tokenPlaceholder := "leave blank to keep current token"
	if !target.HasCredential {
		tokenPlaceholder = "no token configured"
	}

	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Account ID").Value(&accountID),
		huh.NewSelect[bool]().Title("Enabled").Options(huh.NewOption("Yes", true), huh.NewOption("No", false)).Value(&enabled),
		huh.NewSelect[bool]().Title("Required").Options(huh.NewOption("No", false), huh.NewOption("Yes", true)).Value(&required),
		huh.NewSelect[bool]().Title("Send reasoning").Options(huh.NewOption("No", false), huh.NewOption("Yes", true)).Value(&sendReasoning),
		huh.NewSelect[bool]().Title("Send tool events").Options(huh.NewOption("No", false), huh.NewOption("Yes", true)).Value(&sendToolEvents),
		huh.NewSelect[bool]().Title("Require mention").Options(huh.NewOption("No", false), huh.NewOption("Yes", true)).Value(&requireMention),
		huh.NewInput().Title("Discord bot token").Placeholder(tokenPlaceholder).Value(&botToken),
		huh.NewInput().Title("Guild allowlist").Placeholder("comma-separated guild ids").Value(&guildAllowlist),
		huh.NewInput().Title("Allowed channel IDs").Placeholder("comma-separated channel ids").Value(&allowedChannelIDs),
		huh.NewInput().Title("Allowed user IDs").Placeholder("comma-separated user ids").Value(&allowedUserIDs),
		huh.NewSelect[bool]().Title("Allow DMs").Options(huh.NewOption("No", false), huh.NewOption("Yes", true)).Value(&dmEnabled),
	))

	submitted, err := bridge.ShowValidatedHuhForm(form, func() error {
		if strings.TrimSpace(accountID) == "" {
			return fmt.Errorf("Account ID is required.")
		}
		if enabled && !target.HasCredential && strings.TrimSpace(botToken) == "" {
			return fmt.Errorf("Discord bot token is required for enabled channels.")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !submitted {
		return ErrCancelled
	}

	updated, err := b.UpdateChannel(target.ID, &models.UpdateGatewayChannelDto{
		AccountID:      strings.TrimSpace(accountID),
		Enabled:        &enabled,
		Required:       &required,
		SendReasoning:  &sendReasoning,
		SendToolEvents: &sendToolEvents,
		RequireMention: &requireMention,
		Discord: &models.GatewayDiscordChannelConfig{
			BotToken:          strings.TrimSpace(botToken),
			GuildAllowlist:    splitCSV(guildAllowlist),
			AllowedChannelIDs: splitCSV(allowedChannelIDs),
			AllowedUserIDs:    splitCSV(allowedUserIDs),
			DMEnabled:         dmEnabled,
		},
	})
	if err != nil {
		return err
	}
	bridge.Success("Gateway channel updated: %s", updated.ID)
	return nil
}

func splitCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
