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
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/chat"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/openai/openai-go/v3"
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
	return &models.ChatSessionDto{
		ID:        session.ID,
		CreatedAt: session.CreatedAt,
		UpdatedAt: session.UpdatedAt,
	}, nil
}

func (s *ChatService) GetSession(ctx context.Context, sessionID uuid.UUID) (*models.ChatSessionDto, error) {
	session, err := gorm.G[models.ChatSession](s.db).
		Where("id = ? AND deleted_at IS NULL", sessionID).
		First(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find chat session", "error", err, "session_id", sessionID)
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

	messageDtos := lo.Map(chatMessages, func(cm *models.ChatMessage, _ int) models.ChatMessageDto {
		return models.ChatMessageDto{
			ID:        cm.ID,
			Role:      cm.Role,
			Content:   cm.Content,
			CreatedAt: cm.CreatedAt,
		}
	})

	return &models.ChatSessionDto{
		ID:            session.ID,
		TokenConsumed: session.TokenConsumed,
		Messages:      messageDtos,
		CreatedAt:     session.CreatedAt,
		UpdatedAt:     session.UpdatedAt,
	}, nil
}

func (s *ChatService) Chat(ctx context.Context, sessionID uuid.UUID, message string) (*models.ChatMessageDto, error) {
	session, err := gorm.G[models.ChatSession](s.db).
		Where("id = ? AND deleted_at IS NULL", sessionID).
		First(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find chat session", "error", err, "session_id", sessionID)
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

	messages := lo.Map(chatMessages, func(cm *models.ChatMessage, _ int) openai.ChatCompletionMessageParamUnion {
		if cm.Role == models.RoleUser {
			return openai.UserMessage(cm.Content)
		}
		return openai.AssistantMessage(cm.Content)
	})
	messages = append(messages, openai.UserMessage(message))

	newUserMessage := models.ChatMessage{
		ID:        uuid.New(),
		SessionID: session.ID,
		Role:      models.RoleUser,
		Content:   message,
	}

	if err := gorm.G[models.ChatMessage](s.db).Create(ctx, &newUserMessage); err != nil {
		slog.ErrorContext(ctx, "failed to save user message", "error", err, "session_id", sessionID)
		return nil, err
	}

	response, token, err := s.chatExecutor.Chat(ctx, messages)
	if err != nil {
		slog.ErrorContext(ctx, "chat completion failed", "error", err, "session_id", sessionID)
		return nil, err
	}

	newAssistantMessage := models.ChatMessage{
		ID:        uuid.New(),
		SessionID: session.ID,
		Role:      models.RoleAssistant,
		Content:   response,
	}

	if err := gorm.G[models.ChatMessage](s.db).Create(ctx, &newAssistantMessage); err != nil {
		slog.ErrorContext(ctx, "failed to save assistant message", "error", err, "session_id", sessionID)
		return nil, err
	}

	session.TokenConsumed += token
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
