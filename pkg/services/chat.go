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

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/chat"
	"github.com/masteryyh/agenty/pkg/chat/provider"
	"github.com/masteryyh/agenty/pkg/conn"
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
	session := &models.ChatSession{}
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

		return provider.Message{
			Role:       string(cm.Role),
			Content:    cm.Content,
			ToolCalls:  toolCalls,
			ToolResult: toolResult,
		}
	})
	messages = append(messages, provider.Message{
		Role:    provider.RoleUser,
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
		Model:    model.Name,
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

		return models.ChatMessage{
			SessionID:   session.ID,
			Role:        models.MessageRole(m.Role),
			Content:     m.Content,
			ToolCalls:   datatypes.JSON(rawCalls),
			ToolResults: datatypes.JSON(rawCallResult),
			ModelID:     model.ID,
		}
	})

	if err := s.db.WithContext(ctx).Create(&newMessages).Error; err != nil {
		slog.ErrorContext(ctx, "failed to save assistant message", "error", err, "sessionId", sessionID)
		return nil, err
	}

	session.TokenConsumed += result.TotalToken
	if _, err := gorm.G[models.ChatSession](s.db).
		Where("id = ?", session.ID).
		Update(ctx, "token_consumed", session.TokenConsumed); err != nil {
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

		return &models.ChatMessageDto{
			ID:         m.ID,
			Role:       m.Role,
			Content:    m.Content,
			ToolCalls:  toolCalls,
			ToolResult: toolResult,
			CreatedAt:  m.CreatedAt,
		}
	})

	if s.memoryService.IsEnabled() {
		safe.GoSafeWithCtx("auto-memory", ctx, func(bgCtx context.Context) {
			s.evaluateAndSaveMemory(bgCtx, data.Message, result.Messages, chatProvider, model)
		})
	}

	return messageDtos, nil
}

const memoryEvalPrompt = `You are a memory evaluation assistant. Analyze the following conversation and determine if it contains information worth remembering for future conversations.

Worth remembering includes:
- User preferences, habits, or personal information
- Important facts or decisions made
- Technical details or solutions discussed
- Key context that would be useful in future conversations

If the conversation contains memorable information, respond with EXACTLY this format:
SAVE: <a clear, concise summary of the information to remember>

If nothing is worth remembering, respond with EXACTLY:
SKIP

Only extract the most important information. Be concise.`

func (s *ChatService) evaluateAndSaveMemory(ctx context.Context, userMessage string, assistantMessages []provider.Message, chatProvider models.ModelProvider, model models.Model) {
	var sb strings.Builder
	sb.WriteString("User: ")
	sb.WriteString(userMessage)
	sb.WriteString("\n")
	for _, msg := range assistantMessages {
		if msg.Role == provider.RoleAssistant && msg.Content != "" {
			sb.WriteString("Assistant: ")
			sb.WriteString(msg.Content)
			sb.WriteString("\n")
		}
	}

	evalMessages := []provider.Message{
		{Role: provider.RoleSystem, Content: memoryEvalPrompt},
		{Role: provider.RoleUser, Content: sb.String()},
	}

	result, err := s.chatExecutor.Chat(ctx, &chat.ChatParams{
		BaseURL:  chatProvider.BaseURL,
		APIKey:   chatProvider.APIKey,
		Model:    model.Name,
		Messages: evalMessages,
		APIType:  chatProvider.Type,
	})
	if err != nil {
		slog.ErrorContext(ctx, "memory evaluation failed", "error", err)
		return
	}

	for _, msg := range result.Messages {
		if msg.Role == provider.RoleAssistant && strings.HasPrefix(msg.Content, "SAVE: ") {
			content := strings.TrimPrefix(msg.Content, "SAVE: ")
			content = strings.TrimSpace(content)
			if content != "" {
				if _, err := s.memoryService.SaveMemory(ctx, content); err != nil {
					slog.ErrorContext(ctx, "failed to auto-save memory", "error", err)
				} else {
					slog.InfoContext(ctx, "auto-saved memory", "content", content)
				}
			}
			return
		}
	}
}
