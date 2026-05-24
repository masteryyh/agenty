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

package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
	gatewaychannel "github.com/masteryyh/agenty/pkg/gateway/channel"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/safe"
)

const discordMessageLimit = 2000

type Adapter struct {
	channelID      string
	channelType    models.ChannelType
	accountID      string
	cfg            models.GatewayDiscordChannelConfig
	sendReasoning  bool
	sendToolEvents bool
	session        *discordgo.Session
	cancel         context.CancelFunc
	mu             sync.RWMutex
}

func New(ch *models.GatewayChannel) (*Adapter, error) {
	if ch == nil {
		return nil, fmt.Errorf("gateway channel is required")
	}
	cfg := ch.DiscordConfig()
	if cfg == nil {
		return nil, fmt.Errorf("discord config is required")
	}
	token := strings.TrimSpace(cfg.BotToken)
	if token == "" {
		return nil, fmt.Errorf("discord bot token is required")
	}
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}
	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentsMessageContent
	return &Adapter{
		channelID:      ch.ID,
		channelType:    ch.Type,
		accountID:      ch.AccountID,
		cfg:            *cfg,
		sendReasoning:  ch.SendReasoning,
		sendToolEvents: ch.SendToolEvents,
		session:        session,
	}, nil
}

func (a *Adapter) ID() string               { return a.channelID }
func (a *Adapter) Type() models.ChannelType { return a.channelType }
func (a *Adapter) AccountID() string        { return a.accountID }

func (a *Adapter) Start(ctx context.Context, handler gatewaychannel.InboundHandler) error {
	a.mu.Lock()
	if a.session == nil {
		a.mu.Unlock()
		return fmt.Errorf("discord session is not running")
	}
	runCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	session := a.session
	a.mu.Unlock()

	session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m == nil || m.Author == nil || m.Author.Bot {
			return
		}
		if !a.allowMessage(m) {
			return
		}
		botID := a.botUserID()
		mentionsBot := botID != "" && messageMentionsBot(m.Mentions, botID)
		text := strings.TrimSpace(stripBotMention(m.Content, botID))
		if text == "" {
			return
		}
		inbound := gatewaychannel.InboundMessage{
			ID:             m.ID,
			ChannelID:      a.channelID,
			ChannelType:    a.channelType,
			AccountID:      a.accountID,
			ConversationID: m.ChannelID,
			SenderID:       m.Author.ID,
			SenderName:     m.Author.Username,
			Text:           text,
			MentionsBot:    mentionsBot || m.GuildID == "",
			ReceivedAt:     time.Now(),
		}
		safe.GoOnce("gateway-discord-inbound", func() {
			if err := handler.HandleInbound(runCtx, a, inbound); err != nil {
				slog.ErrorContext(runCtx, "failed to handle discord inbound message", "error", err, "channelId", a.channelID, "messageId", m.ID)
			}
		})
	})
	if err := session.Open(); err != nil {
		cancel()
		a.mu.Lock()
		a.cancel = nil
		a.mu.Unlock()
		return err
	}
	return nil
}

func (a *Adapter) Stop(context.Context) error {
	a.mu.Lock()
	cancel := a.cancel
	a.cancel = nil
	session := a.session
	a.session = nil
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if session == nil {
		return nil
	}
	return session.Close()
}

func (a *Adapter) Send(ctx context.Context, event gatewaychannel.OutboundEvent) error {
	text := strings.TrimSpace(event.Text)
	if text == "" {
		return nil
	}
	switch event.Type {
	case gatewaychannel.OutboundMessageDone, gatewaychannel.OutboundError:
	case gatewaychannel.OutboundToolCallStart, gatewaychannel.OutboundToolCallDone, gatewaychannel.OutboundToolResult:
		if !a.sendToolEvents {
			return nil
		}
	case gatewaychannel.OutboundReasoning:
		if !a.sendReasoning {
			return nil
		}
	default:
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	a.mu.RLock()
	session := a.session
	a.mu.RUnlock()
	if session == nil {
		return fmt.Errorf("discord session is not running")
	}
	for _, chunk := range splitMessage(text, discordMessageLimit) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if _, err := session.ChannelMessageSend(event.ConversationID, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) allowMessage(m *discordgo.MessageCreate) bool {
	if len(a.cfg.AllowedUserIDs) > 0 && !contains(a.cfg.AllowedUserIDs, m.Author.ID) {
		return false
	}
	if m.GuildID == "" {
		return a.cfg.DMEnabled
	}
	if len(a.cfg.GuildAllowlist) > 0 && !contains(a.cfg.GuildAllowlist, m.GuildID) {
		return false
	}
	if len(a.cfg.AllowedChannelIDs) > 0 && !contains(a.cfg.AllowedChannelIDs, m.ChannelID) {
		return false
	}
	return true
}

func (a *Adapter) botUserID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.session == nil || a.session.State == nil || a.session.State.User == nil {
		return ""
	}
	return a.session.State.User.ID
}

func messageMentionsBot(mentions []*discordgo.User, botID string) bool {
	if botID == "" {
		return false
	}
	for _, mention := range mentions {
		if mention != nil && mention.ID == botID {
			return true
		}
	}
	return false
}

func stripBotMention(content, botID string) string {
	if botID == "" {
		return content
	}
	content = strings.ReplaceAll(content, "<@"+botID+">", "")
	content = strings.ReplaceAll(content, "<@!"+botID+">", "")
	return content
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func splitMessage(text string, limit int) []string {
	if limit <= 0 || utf8.RuneCountInString(text) <= limit {
		return []string{text}
	}
	chunks := make([]string, 0, len(text)/limit+1)
	start := 0
	count := 0
	for i := range text {
		if count == limit {
			chunks = append(chunks, text[start:i])
			start = i
			count = 0
		}
		count++
	}
	if start < len(text) {
		chunks = append(chunks, text[start:])
	}
	return chunks
}
