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

package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/chat"
	"github.com/masteryyh/agenty/pkg/chat/provider"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"github.com/samber/lo"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ChatService struct {
	chatExecutor  *chat.ChatExecutor
	db            *gorm.DB
	memoryService *MemoryService
}

var (
	chatService *ChatService
	chatOnce    sync.Once
)

func GetChatService() *ChatService {
	chatOnce.Do(func() {
		chatService = &ChatService{
			chatExecutor:  chat.GetChatExecutor(),
			db:            conn.GetDB(),
			memoryService: GetMemoryService(),
		}
	})
	return chatService
}

func (s *ChatService) CreateSession(ctx context.Context) (*models.ChatSessionDto, error) {
	defaultModel, err := gorm.G[models.Model](s.db).
		Where("default_model IS true AND deleted_at IS NULL").
		First(ctx)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			slog.ErrorContext(ctx, "failed to find default model for new session", "error", err)
			return nil, err
		}
	}

	if defaultModel.ID == uuid.Nil {
		defaultModel, err = gorm.G[models.Model](s.db).
			Where("deleted_at IS NULL").
			Order("created_at DESC").
			First(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "failed to find any model for new session", "error", err)
			return nil, err
		}
	}

	defaultModelProvider, err := gorm.G[models.ModelProvider](s.db).
		Where("id = ? AND deleted_at IS NULL", defaultModel.ProviderID).
		First(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find provider for default model", "error", err, "providerId", defaultModel.ProviderID)
		return nil, err
	}

	if defaultModelProvider.APIKey == "" {
		return nil, customerrors.ErrProviderNotConfigured
	}

	session := &models.ChatSession{
		LastUsedModel: defaultModel.ID,
	}
	if err := gorm.G[models.ChatSession](s.db).Create(ctx, session); err != nil {
		slog.ErrorContext(ctx, "failed to create chat session", "error", err)
		return nil, err
	}
	return session.ToDto(nil), nil
}

func (s *ChatService) GetSession(ctx context.Context, sessionID uuid.UUID) (*models.ChatSessionDto, error) {
	session, err := gorm.G[models.ChatSession](s.db).
		Where("id = ? AND deleted_at IS NULL", sessionID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrSessionNotFound
		}

		slog.ErrorContext(ctx, "failed to find chat session", "error", err, "sessionId", sessionID)
		return nil, err
	}

	chatMessages, err := gorm.G[*models.ChatMessage](s.db).
		Where("session_id = ? AND deleted_at IS NULL", session.ID).
		Order("created_at ASC").
		Find(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find chat messages", "error", err, "sessionId", session.ID)
		return nil, err
	}
	if len(chatMessages) == 0 {
		return session.ToDto(nil), nil
	}

	modelIds := lo.Uniq(lo.Map(chatMessages, func(cm *models.ChatMessage, _ int) uuid.UUID {
		return cm.ModelID
	}))

	chatModels, err := gorm.G[models.Model](s.db).
		Where("id IN ? AND deleted_at IS NULL", modelIds).
		Find(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find models for chat messages", "error", err, "modelIds", modelIds)
		return nil, err
	}
	modelMap := lo.Associate(chatModels, func(m models.Model) (uuid.UUID, *models.ModelDto) {
		return m.ID, m.ToDto(nil)
	})

	messageDtos := lo.Map(chatMessages, func(cm *models.ChatMessage, _ int) models.ChatMessageDto {
		return *cm.ToDto(modelMap[cm.ModelID])
	})
	return session.ToDto(messageDtos), nil
}

func (s *ChatService) GetLastSession(ctx context.Context) (*models.ChatSessionDto, error) {
	session, err := gorm.G[models.ChatSession](s.db).
		Where("deleted_at IS NULL").
		Order("updated_at DESC").
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		slog.ErrorContext(ctx, "failed to find last chat session", "error", err)
		return nil, err
	}

	chatMessages, err := gorm.G[*models.ChatMessage](s.db).
		Where("session_id = ? AND deleted_at IS NULL", session.ID).
		Order("created_at ASC").
		Find(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find chat messages", "error", err, "sessionId", session.ID)
		return nil, err
	}
	if len(chatMessages) == 0 {
		return session.ToDto(nil), nil
	}

	modelIds := lo.Uniq(lo.Map(chatMessages, func(cm *models.ChatMessage, _ int) uuid.UUID {
		return cm.ModelID
	}))

	chatModels, err := gorm.G[models.Model](s.db).
		Where("id IN ? AND deleted_at IS NULL", modelIds).
		Find(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find models for chat messages", "error", err, "modelIds", modelIds)
		return nil, err
	}
	modelMap := lo.Associate(chatModels, func(m models.Model) (uuid.UUID, *models.ModelDto) {
		return m.ID, m.ToDto(nil)
	})

	messageDtos := lo.Map(chatMessages, func(cm *models.ChatMessage, _ int) models.ChatMessageDto {
		return *cm.ToDto(modelMap[cm.ModelID])
	})
	return session.ToDto(messageDtos), nil
}

func (s *ChatService) ListSessions(ctx context.Context, request *pagination.PageRequest) (*pagination.PagedResponse[models.ChatSessionDto], error) {
	var dtos []models.ChatSessionDto
	var total int64

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		sessions, err := gorm.G[models.ChatSession](tx).
			Where("deleted_at IS NULL").
			Offset((request.Page - 1) * request.PageSize).
			Limit(request.PageSize).
			Order("updated_at DESC").
			Find(ctx)
		if err != nil {
			return fmt.Errorf("failed to find sessions: %w", err)
		}

		countResult, err := gorm.G[models.ChatSession](tx).
			Where("deleted_at IS NULL").
			Count(ctx, "id")
		if err != nil {
			return fmt.Errorf("failed to count sessions: %w", err)
		}
		total = countResult

		dtos = lo.Map(sessions, func(s models.ChatSession, _ int) models.ChatSessionDto {
			return *s.ToDto(nil)
		})
		return nil
	}); err != nil {
		slog.ErrorContext(ctx, "failed to list chat sessions", "error", err)
		return nil, err
	}

	return &pagination.PagedResponse[models.ChatSessionDto]{
		Total:    total,
		PageSize: request.PageSize,
		Page:     request.Page,
		Data:     dtos,
	}, nil
}

func (s *ChatService) Chat(ctx context.Context, sessionID uuid.UUID, data *models.ChatDto) ([]*models.ChatMessageDto, error) {
	session, err := gorm.G[models.ChatSession](s.db).
		Where("id = ? AND deleted_at IS NULL", sessionID).
		First(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find chat session", "error", err, "sessionId", sessionID)
		return nil, err
	}

	model, err := gorm.G[models.Model](s.db).
		Where("id = ? AND deleted_at IS NULL", data.ModelID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrModelNotFound
		}
		slog.ErrorContext(ctx, "failed to find model", "error", err, "modelId", data.ModelID)
		return nil, err
	}

	chatProvider, err := gorm.G[models.ModelProvider](s.db).
		Where("id = ? AND deleted_at IS NULL", model.ProviderID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrProviderNotFound
		}
		slog.ErrorContext(ctx, "failed to find provider", "error", err, "providerId", model.ProviderID)
		return nil, err
	}

	if chatProvider.APIKey == "" {
		return nil, customerrors.ErrProviderNotConfigured
	}

	chatMessages, err := gorm.G[*models.ChatMessage](s.db).
		Where("session_id = ? AND deleted_at IS NULL", session.ID).
		Order("created_at ASC").
		Find(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find chat messages", "error", err, "sessionId", sessionID)
		return nil, err
	}

	messages := lo.Map(chatMessages, func(cm *models.ChatMessage, _ int) provider.Message {
		var toolCalls []models.ToolCall
		if len(cm.ToolCalls) > 0 {
			if err := json.Unmarshal(cm.ToolCalls, &toolCalls); err != nil {
				slog.ErrorContext(ctx, "failed to unmarshal tool calls", "error", err, "sessionId", sessionID, "messageId", cm.ID)
			}
		}

		var toolResult *models.ToolResult
		if len(cm.ToolResults) > 0 {
			var tr models.ToolResult
			if err := json.Unmarshal(cm.ToolResults, &tr); err != nil {
				slog.ErrorContext(ctx, "failed to unmarshal tool result", "error", err, "sessionId", sessionID, "messageId", cm.ID)
			} else {
				toolResult = &tr
			}
		}

		msg := provider.Message{
			Role:       cm.Role,
			Content:    cm.Content,
			ToolCalls:  toolCalls,
			ToolResult: toolResult,
		}

		if data.ModelID == cm.ModelID {
			
		}

		return msg
	})
	messages = append(messages, provider.Message{
		Role:    models.RoleUser,
		Content: data.Message,
	})

	newUserMessage := models.ChatMessage{
		SessionID: session.ID,
		Role:      models.RoleUser,
		Content:   data.Message,
		ModelID:   model.ID,
	}

	if err := gorm.G[models.ChatMessage](s.db).Create(ctx, &newUserMessage); err != nil {
		slog.ErrorContext(ctx, "failed to save user message", "error", err, "sessionId", sessionID)
		return nil, err
	}

	result, err := s.chatExecutor.Chat(ctx, &chat.ChatParams{
		BaseURL:  chatProvider.BaseURL,
		APIKey:   chatProvider.APIKey,
		Model:    model.Code,
		Messages: messages,
		APIType:  chatProvider.Type,
	})
	if err != nil {
		slog.ErrorContext(ctx, "chat completion failed", "error", err, "sessionId", sessionID)
		return nil, err
	}

	newMessages := lo.Map(result.Messages, func(m provider.Message, _ int) models.ChatMessage {
		var rawCalls []byte
		if len(m.ToolCalls) > 0 {
			if data, err := json.Marshal(m.ToolCalls); err != nil {
				slog.ErrorContext(ctx, "failed to marshal tool calls", "error", err, "sessionId", sessionID)
			} else {
				rawCalls = data
			}
		}

		var rawCallResult []byte
		if m.ToolResult != nil {
			if data, err := json.Marshal(m.ToolResult); err != nil {
				slog.ErrorContext(ctx, "failed to marshal tool result", "error", err, "sessionId", sessionID)
			} else {
				rawCallResult = data
			}
		}

		chatMsg := models.ChatMessage{
			SessionID:   session.ID,
			Role:        models.MessageRole(m.Role),
			Content:     m.Content,
			ToolCalls:   datatypes.JSON(rawCalls),
			ToolResults: datatypes.JSON(rawCallResult),
			ModelID:     model.ID,
		}

		// if chatProvider.Type == models.APITypeKimi && m.ReasoningContent != "" {
		// 	specificData := models.ProviderSpecificData{
		// 		KimiReasoningContent: m.ReasoningContent,
		// 	}
		// 	if data, err := json.Marshal(specificData); err == nil {
		// 		chatMsg.ProviderSpecifics = datatypes.JSON(data)
		// 	}
		// }
		return chatMsg
	})

	if err := s.db.WithContext(ctx).Create(&newMessages).Error; err != nil {
		slog.ErrorContext(ctx, "failed to save assistant message", "error", err, "sessionId", sessionID)
		return nil, err
	}

	session.TokenConsumed += result.TotalToken
	session.LastUsedModel = model.ID
	if _, err := gorm.G[models.ChatSession](s.db).
		Where("id = ?", session.ID).
		Updates(ctx, session); err != nil {
		slog.ErrorContext(ctx, "failed to update token consumed", "error", err, "sessionId", sessionID)
		return nil, err
	}

	messageDtos := lo.Map(newMessages, func(m models.ChatMessage, _ int) *models.ChatMessageDto {
		var toolCalls []models.ToolCall
		if len(m.ToolCalls) > 0 {
			if err := json.Unmarshal(m.ToolCalls, &toolCalls); err != nil {
				slog.ErrorContext(ctx, "failed to unmarshal tool calls", "error", err, "sessionId", sessionID, "messageId", m.ID)
			}
		}

		var toolResult *models.ToolResult
		if len(m.ToolResults) > 0 {
			var tr models.ToolResult
			if err := json.Unmarshal(m.ToolResults, &tr); err != nil {
				slog.ErrorContext(ctx, "failed to unmarshal tool result", "error", err, "sessionId", sessionID, "messageId", m.ID)
			} else {
				toolResult = &tr
			}
		}

		var providerSpecifics *models.ProviderSpecificData
		if len(m.ProviderSpecifics) > 0 {
			var ps models.ProviderSpecificData
			if err := json.Unmarshal(m.ProviderSpecifics, &ps); err != nil {
				slog.ErrorContext(ctx, "failed to unmarshal provider specifics", "error", err, "sessionId", sessionID, "messageId", m.ID)
			} else {
				providerSpecifics = &ps
			}
		}

		return &models.ChatMessageDto{
			ID:                m.ID,
			Role:              m.Role,
			Content:           m.Content,
			ToolCalls:         toolCalls,
			ToolResult:        toolResult,
			ProviderSpecifics: providerSpecifics,
			CreatedAt:         m.CreatedAt,
		}
	})

	if s.memoryService.IsEnabled() {
		safe.GoSafeWithCtx("auto-memory", ctx, func(bgCtx context.Context) {
			s.evaluateAndSaveMemory(bgCtx, data.Message, result.Messages, chatProvider, model)
		})
	}

	return messageDtos, nil
}

func (s *ChatService) evaluateAndSaveMemory(ctx context.Context, userMessage string, assistantMessages []provider.Message, chatProvider models.ModelProvider, model models.Model) {
	var sb strings.Builder
	sb.WriteString("User: ")
	sb.WriteString(userMessage)
	sb.WriteString("\n")
	for _, msg := range assistantMessages {
		if msg.Role == models.RoleAssistant && msg.Content != "" {
			sb.WriteString("Assistant: ")
			sb.WriteString(msg.Content)
			sb.WriteString("\n")
		}
	}

	var promptBuilder strings.Builder
	if err := consts.MemoryEvalPrompt.Execute(&promptBuilder, map[string]any{
		"DateTime": time.Now().Format(time.RFC3339),
	}); err != nil {
		slog.ErrorContext(ctx, "failed to execute memory evaluation prompt", "error", err)
		return
	}

	evalMessages := []provider.Message{
		{Role: models.RoleSystem, Content: promptBuilder.String()},
		{Role: models.RoleUser, Content: sb.String()},
	}

	responseFormat := &provider.ResponseFormat{Type: "json_object"}
	if chatProvider.Type == models.APITypeOpenAI {
		responseFormat = &provider.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &provider.JSONSchemaFormat{
				Name:        "memory_evaluation",
				Description: "Extract facts from conversation for long-term memory",
				Strict:      true,
				Schema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"facts": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "string",
							},
						},
					},
					"required":             []string{"facts"},
					"additionalProperties": false,
				},
			},
		}
	}

	result, err := s.chatExecutor.Chat(ctx, &chat.ChatParams{
		BaseURL:        chatProvider.BaseURL,
		APIKey:         chatProvider.APIKey,
		Model:          model.Code,
		Messages:       evalMessages,
		APIType:        chatProvider.Type,
		ResponseFormat: responseFormat,
	})
	if err != nil {
		slog.ErrorContext(ctx, "memory evaluation failed", "error", err)
		return
	}

	for _, msg := range result.Messages {
		if msg.Role == models.RoleAssistant && msg.Content != "" {
			var evalResult struct {
				Facts []string `json:"facts"`
			}
			if err := json.Unmarshal([]byte(msg.Content), &evalResult); err != nil {
				slog.ErrorContext(ctx, "failed to parse memory evaluation result", "error", err, "content", msg.Content)
				return
			}

			if len(evalResult.Facts) == 0 {
				return
			}

			for _, fact := range evalResult.Facts {
				fact = strings.TrimSpace(fact)
				if fact != "" {
					if _, err := s.memoryService.SaveMemory(ctx, fact); err != nil {
						slog.ErrorContext(ctx, "failed to auto-save memory", "error", err, "fact", fact)
					} else {
						slog.InfoContext(ctx, "auto-saved memory", "fact", fact)
					}
				}
			}
			return
		}
	}
}
