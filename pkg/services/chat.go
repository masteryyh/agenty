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
	"sync"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/chat"
	"github.com/masteryyh/agenty/pkg/chat/provider"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

type ChatService struct {
	chatExecutor *chat.ChatExecutor
	db           *gorm.DB
}

var (
	chatService *ChatService
	chatOnce    sync.Once
)

func GetChatService() *ChatService {
	chatOnce.Do(func() {
		chatService = &ChatService{
			chatExecutor: chat.GetChatExecutor(),
			db:           conn.GetDB(),
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

		slog.ErrorContext(ctx, "failed to find chat session", "error", err, "session_id", sessionID)
		return nil, err
	}

	chatMessages, err := gorm.G[*models.ChatMessage](s.db).
		Where("session_id = ? AND deleted_at IS NULL", session.ID).
		Order("created_at ASC").
		Find(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find chat messages", "error", err, "session_id", session.ID)
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
		slog.ErrorContext(ctx, "failed to find models for chat messages", "error", err, "model_ids", modelIds)
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

func (s *ChatService) Chat(ctx context.Context, sessionID uuid.UUID, data *models.ChatDto) (*models.ChatMessageDto, error) {
	session, err := gorm.G[models.ChatSession](s.db).
		Where("id = ? AND deleted_at IS NULL", sessionID).
		First(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find chat session", "error", err, "session_id", sessionID)
		return nil, err
	}

	model, err := gorm.G[models.Model](s.db).
		Where("id = ? AND deleted_at IS NULL", data.ModelID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrModelNotFound
		}
		slog.ErrorContext(ctx, "failed to find model", "error", err, "model_id", data.ModelID)
		return nil, err
	}

	chatProvider, err := gorm.G[models.ModelProvider](s.db).
		Where("id = ? AND deleted_at IS NULL", model.ProviderID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrProviderNotFound
		}
		slog.ErrorContext(ctx, "failed to find provider", "error", err, "provider_id", model.ProviderID)
		return nil, err
	}

	chatMessages, err := gorm.G[*models.ChatMessage](s.db).
		Where("session_id = ? AND deleted_at IS NULL", session.ID).
		Order("created_at ASC").
		Find(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find chat messages", "error", err, "session_id", sessionID)
		return nil, err
	}

	messages := lo.Map(chatMessages, func(cm *models.ChatMessage, _ int) provider.Message {
		return provider.Message{
			Role:    string(cm.Role),
			Content: cm.Content,
		}
	})
	messages = append(messages, provider.Message{
		Role:    "user",
		Content: data.Message,
	})

	newUserMessage := models.ChatMessage{
		SessionID: session.ID,
		Role:      models.RoleUser,
		Content:   data.Message,
		ModelID:   model.ID,
	}

	if err := gorm.G[models.ChatMessage](s.db).Create(ctx, &newUserMessage); err != nil {
		slog.ErrorContext(ctx, "failed to save user message", "error", err, "session_id", sessionID)
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
		slog.ErrorContext(ctx, "chat completion failed", "error", err, "session_id", sessionID)
		return nil, err
	}

	newAssistantMessage := models.ChatMessage{
		SessionID: session.ID,
		Role:      models.RoleAssistant,
		Content:   result.Content,
		ModelID:   model.ID,
	}

	if err := gorm.G[models.ChatMessage](s.db).Create(ctx, &newAssistantMessage); err != nil {
		slog.ErrorContext(ctx, "failed to save assistant message", "error", err, "session_id", sessionID)
		return nil, err
	}

	session.TokenConsumed += result.TotalToken
	if _, err := gorm.G[models.ChatSession](s.db).
		Where("id = ?", session.ID).
		Update(ctx, "token_consumed", session.TokenConsumed); err != nil {
		slog.ErrorContext(ctx, "failed to update token consumed", "error", err, "session_id", sessionID)
		return nil, err
	}

	return &models.ChatMessageDto{
		ID:        newAssistantMessage.ID,
		Role:      newAssistantMessage.Role,
		Content:   newAssistantMessage.Content,
		CreatedAt: newAssistantMessage.CreatedAt,
	}, nil
}
