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
	"log/slog"
	"time"

	json "github.com/bytedance/sonic"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
	RoleSystem    MessageRole = "system"
)

type ChatMessage struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuidv7()"`
	SessionID         uuid.UUID      `gorm:"type:uuid;not null"`
	Role              MessageRole    `gorm:"type:varchar(50);not null"`
	Content           string         `gorm:"type:text"`
	ToolCalls         datatypes.JSON `gorm:"type:jsonb"`
	ToolResults       datatypes.JSON `gorm:"type:jsonb"`
	ModelID           uuid.UUID      `gorm:"type:uuid;not null"`
	ReasoningContent  string         `gorm:"type:text"`
	ProviderSpecifics datatypes.JSON `gorm:"type:jsonb"`
	CreatedAt         time.Time      `gorm:"autoCreateTime:milli"`
	DeletedAt         *time.Time
}

func (ChatMessage) TableName() string {
	return "chat_messages"
}

func (m *ChatMessage) ToDto(model *ModelDto) *ChatMessageDto {
	var toolCalls []ToolCall
	if len(m.ToolCalls) > 0 {
		if err := json.Unmarshal(m.ToolCalls, &toolCalls); err != nil {
			slog.Error("failed to unmarshal tool calls", "error", err, "sessionId", m.SessionID, "messageId", m.ID)
		}
	}

	var toolResult *ToolResult
	if len(m.ToolResults) > 0 {
		var tr ToolResult
		if err := json.Unmarshal(m.ToolResults, &tr); err != nil {
			slog.Error("failed to unmarshal tool result", "error", err, "sessionId", m.SessionID, "messageId", m.ID)
		} else {
			toolResult = &tr
		}
	}

	var providerSpecifics *ProviderSpecificData
	if len(m.ProviderSpecifics) > 0 {
		var ps ProviderSpecificData
		if err := json.Unmarshal(m.ProviderSpecifics, &ps); err != nil {
			slog.Error("failed to unmarshal provider specifics", "error", err, "sessionId", m.SessionID, "messageId", m.ID)
		} else {
			providerSpecifics = &ps
		}
	}

	dto := &ChatMessageDto{
		ID:                m.ID,
		Role:              m.Role,
		Content:           m.Content,
		ToolCalls:         toolCalls,
		ToolResult:        toolResult,
		ProviderSpecifics: providerSpecifics,
		ReasoningContent:  m.ReasoningContent,
		CreatedAt:         m.CreatedAt,
	}

	if model != nil {
		dto.Model = model
	}
	return dto
}

type ChatMessageDto struct {
	ID                uuid.UUID             `json:"id"`
	Role              MessageRole           `json:"role"`
	Content           string                `json:"content"`
	ToolCalls         []ToolCall            `json:"toolCalls,omitempty"`
	ToolResult        *ToolResult           `json:"toolResult,omitempty"`
	Model             *ModelDto             `json:"model,omitempty"`
	ReasoningContent  string                `json:"reasoningContent,omitempty"`
	ProviderSpecifics *ProviderSpecificData `json:"providerSpecifics,omitempty"`
	CreatedAt         time.Time             `json:"createdAt"`
}

type ChatDto struct {
	ModelID       uuid.UUID `json:"modelId" binding:"required"`
	Message       string    `json:"message" binding:"required"`
	Thinking      bool      `json:"thinking"`
	ThinkingLevel string    `json:"thinkingLevel"`
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolResult struct {
	CallID  string `json:"callId"`
	Name    string `json:"name"`
	Content string `json:"content"`
	IsError bool   `json:"isError"`
}

type AnthropicThinkingBlock struct {
	Type      string `json:"type"`
	Thinking  string `json:"summary,omitempty"`
	Signature string `json:"signature,omitempty"`
	Data      string `json:"data,omitempty"`
}

type GeminiThinkingData struct {
	
}

type ProviderSpecificData struct {
}
