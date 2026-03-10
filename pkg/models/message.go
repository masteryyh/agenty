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
	"github.com/masteryyh/agenty/pkg/db"
)

type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
	RoleSystem    MessageRole = "system"
)

type ChatMessageDto struct {
	ID                uuid.UUID             `json:"id"`
	AgentID           uuid.UUID             `json:"agentId"`
	Role              MessageRole           `json:"role"`
	Content           string                `json:"content"`
	ToolCalls         []ToolCall            `json:"toolCalls,omitempty"`
	ToolResult        *ToolResult           `json:"toolResult,omitempty"`
	Model             *ModelDto             `json:"model,omitempty"`
	ReasoningContent  string                `json:"reasoningContent,omitempty"`
	ProviderSpecifics *ProviderSpecificData `json:"providerSpecifics,omitempty"`
	CreatedAt         time.Time             `json:"createdAt"`
}

func ChatMessageRowToDto(row db.ChatMessage, model *ModelDto) *ChatMessageDto {
	var toolCalls []ToolCall
	if row.ToolCalls.Valid && len(row.ToolCalls.RawMessage) > 0 {
		if err := json.Unmarshal(row.ToolCalls.RawMessage, &toolCalls); err != nil {
			slog.Error("failed to unmarshal tool calls", "error", err, "sessionId", row.SessionID, "messageId", row.ID)
		}
	}

	var toolResult *ToolResult
	if row.ToolResults.Valid && len(row.ToolResults.RawMessage) > 0 {
		var tr ToolResult
		if err := json.Unmarshal(row.ToolResults.RawMessage, &tr); err != nil {
			slog.Error("failed to unmarshal tool result", "error", err, "sessionId", row.SessionID, "messageId", row.ID)
		} else {
			toolResult = &tr
		}
	}

	var providerSpecifics *ProviderSpecificData
	if row.ProviderSpecifics.Valid && len(row.ProviderSpecifics.RawMessage) > 0 {
		var ps ProviderSpecificData
		if err := json.Unmarshal(row.ProviderSpecifics.RawMessage, &ps); err != nil {
			slog.Error("failed to unmarshal provider specifics", "error", err, "sessionId", row.SessionID, "messageId", row.ID)
		} else {
			providerSpecifics = &ps
		}
	}

	dto := &ChatMessageDto{
		ID:                row.ID,
		AgentID:           row.AgentID,
		Role:              MessageRole(row.Role),
		Content:           row.Content,
		ToolCalls:         toolCalls,
		ToolResult:        toolResult,
		ProviderSpecifics: providerSpecifics,
		ReasoningContent:  row.ReasoningContent,
		CreatedAt:         row.CreatedAt,
	}

	if model != nil {
		dto.Model = model
	}
	return dto
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
	ThoughtSignature string `json:"thoughtSignature"`
	ThinkingLevel    string `json:"thinkingLevel"`
	Summary          string `json:"summary"`
}

type OpenAIReasoningBlock struct {
	ID               string `json:"id"`
	Summary          string `json:"summary"`
	EncryptedContent string `json:"encryptedContent,omitempty"`
}

type ProviderSpecificData struct {
	AnthropicThinkingBlocks []AnthropicThinkingBlock `json:"anthropicThinkingBlocks,omitempty"`
	GeminiThinkingBlocks    []GeminiThinkingData     `json:"geminiThinkingBlocks,omitempty"`
	OpenAIReasoningBlocks   []OpenAIReasoningBlock   `json:"openaiReasoningBlocks,omitempty"`
}
