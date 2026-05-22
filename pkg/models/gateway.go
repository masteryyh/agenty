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

package models

import (
	"strings"
	"time"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type ChannelType string

const (
	ChannelTypeDiscord  ChannelType = "discord"
	ChannelTypeSlack    ChannelType = "slack"
	ChannelTypeTelegram ChannelType = "telegram"
	ChannelTypeWechat   ChannelType = "wechat"
	ChannelTypeQQ       ChannelType = "qq"
)

type GatewayDiscordChannelConfig struct {
	BotToken          string   `json:"botToken"`
	AppID             string   `json:"appId,omitempty"`
	PublicKey         string   `json:"publicKey,omitempty"`
	GuildAllowlist    []string `json:"guildAllowlist,omitempty"`
	AllowedChannelIDs []string `json:"allowedChannelIds,omitempty"`
	AllowedUserIDs    []string `json:"allowedUserIds,omitempty"`
	DMEnabled         bool     `json:"dmEnabled"`
}

type GatewayChannel struct {
	ID             string
	Type           ChannelType
	AccountID      string
	Enabled        bool
	Required       bool
	SendReasoning  bool
	SendToolEvents bool
	RequireMention bool
	Config         datatypes.JSON
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (GatewayChannel) TableName() string {
	return "gateway_channels"
}

func (c *GatewayChannel) DiscordConfig() *GatewayDiscordChannelConfig {
	if c == nil || c.Type != "discord" || len(c.Config) == 0 {
		return nil
	}
	var cfg GatewayDiscordChannelConfig
	if err := json.Unmarshal(c.Config, &cfg); err != nil {
		return nil
	}
	return &cfg
}

func (c *GatewayChannel) SetDiscordConfig(cfg *GatewayDiscordChannelConfig) error {
	if cfg == nil {
		c.Config = nil
		return nil
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	c.Config = raw
	return nil
}

func (c *GatewayChannel) ToDto() *GatewayChannelDto {
	dto := &GatewayChannelDto{
		ID:             c.ID,
		Type:           c.Type,
		AccountID:      c.AccountID,
		Enabled:        c.Enabled,
		Required:       c.Required,
		SendReasoning:  c.SendReasoning,
		SendToolEvents: c.SendToolEvents,
		RequireMention: c.RequireMention,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}
	if c.Type == ChannelTypeDiscord {
		raw := c.DiscordConfig()
		if raw != nil {
			redacted := *raw
			redacted.BotToken = ""
			dto.Discord = &redacted
			dto.HasCredential = strings.TrimSpace(raw.BotToken) != ""
		}
	}
	return dto
}

type GatewayChannelDto struct {
	ID             string                       `json:"id"`
	Type           ChannelType                  `json:"type"`
	AccountID      string                       `json:"accountId"`
	Enabled        bool                         `json:"enabled"`
	Required       bool                         `json:"required"`
	SendReasoning  bool                         `json:"sendReasoning"`
	SendToolEvents bool                         `json:"sendToolEvents"`
	RequireMention bool                         `json:"requireMention"`
	HasCredential  bool                         `json:"hasCredential"`
	Discord        *GatewayDiscordChannelConfig `json:"discord,omitempty"`
	CreatedAt      time.Time                    `json:"createdAt"`
	UpdatedAt      time.Time                    `json:"updatedAt"`
}

type CreateGatewayChannelDto struct {
	ID             string                       `json:"id" binding:"required,code"`
	Type           string                       `json:"type" binding:"required"`
	AccountID      string                       `json:"accountId" binding:"required"`
	Enabled        *bool                        `json:"enabled"`
	Required       bool                         `json:"required"`
	SendReasoning  bool                         `json:"sendReasoning"`
	SendToolEvents bool                         `json:"sendToolEvents"`
	RequireMention bool                         `json:"requireMention"`
	Discord        *GatewayDiscordChannelConfig `json:"discord,omitempty"`
}

type UpdateGatewayChannelDto struct {
	Type           string                       `json:"type"`
	AccountID      string                       `json:"accountId"`
	Enabled        *bool                        `json:"enabled"`
	Required       *bool                        `json:"required"`
	SendReasoning  *bool                        `json:"sendReasoning"`
	SendToolEvents *bool                        `json:"sendToolEvents"`
	RequireMention *bool                        `json:"requireMention"`
	Discord        *GatewayDiscordChannelConfig `json:"discord,omitempty"`
}

type AgentGatewayBinding struct {
	ID             uuid.UUID
	AgentID        uuid.UUID
	ChannelID      string
	ChannelType    ChannelType
	AccountID      string
	DefaultModelID *uuid.UUID
	Enabled        bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (AgentGatewayBinding) TableName() string {
	return "agent_gateway_bindings"
}

func (b *AgentGatewayBinding) ToDto() *AgentGatewayBindingDto {
	return &AgentGatewayBindingDto{
		ID:             b.ID,
		AgentID:        b.AgentID,
		ChannelID:      b.ChannelID,
		ChannelType:    string(b.ChannelType),
		AccountID:      b.AccountID,
		DefaultModelID: b.DefaultModelID,
		Enabled:        b.Enabled,
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
	}
}

type AgentGatewayBindingDto struct {
	ID             uuid.UUID  `json:"id"`
	AgentID        uuid.UUID  `json:"agentId"`
	ChannelID      string     `json:"channelId"`
	ChannelType    string     `json:"channelType"`
	AccountID      string     `json:"accountId"`
	DefaultModelID *uuid.UUID `json:"defaultModelId,omitempty"`
	Enabled        bool       `json:"enabled"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type CreateAgentGatewayBindingDto struct {
	ChannelID      string     `json:"channelId" binding:"required"`
	DefaultModelID *uuid.UUID `json:"defaultModelId"`
	Enabled        *bool      `json:"enabled"`
}

type UpdateAgentGatewayBindingDto struct {
	DefaultModelID    *uuid.UUID `json:"defaultModelId,omitempty"`
	DefaultModelIDSet bool       `json:"-"`
	Enabled           *bool      `json:"enabled,omitempty"`
}

func (d *UpdateAgentGatewayBindingDto) UnmarshalJSON(data []byte) error {
	type updateAgentGatewayBindingDto UpdateAgentGatewayBindingDto
	var dto updateAgentGatewayBindingDto
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*d = UpdateAgentGatewayBindingDto(dto)
	_, d.DefaultModelIDSet = fields["defaultModelId"]
	return nil
}

func (d UpdateAgentGatewayBindingDto) MarshalJSON() ([]byte, error) {
	fields := map[string]any{}
	if d.DefaultModelIDSet {
		fields["defaultModelId"] = d.DefaultModelID
	}
	if d.Enabled != nil {
		fields["enabled"] = d.Enabled
	}
	return json.Marshal(fields)
}

type GatewayConversation struct {
	ID             uuid.UUID
	BindingID      *uuid.UUID
	ChannelID      string
	ChannelType    ChannelType
	AccountID      string
	ConversationID string
	SenderID       string
	AgentID        uuid.UUID
	SessionID      uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (GatewayConversation) TableName() string {
	return "gateway_conversations"
}

type GatewayMessageDelivery struct {
	ID                uuid.UUID
	BindingID         *uuid.UUID
	ChannelID         string
	AccountID         string
	ExternalMessageID string
	ConversationID    string
	AgentID           uuid.UUID
	SessionID         uuid.UUID
	Status            string
	Error             string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (GatewayMessageDelivery) TableName() string {
	return "gateway_message_deliveries"
}
