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
	"database/sql"
	stdjson "encoding/json"
	"errors"
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
	"github.com/masteryyh/agenty/pkg/db"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/samber/lo"
	"github.com/sqlc-dev/pqtype"
)

type ChatService struct {
	chatExecutor *chat.ChatExecutor
	sqlDB        *sql.DB
	queries      *db.Queries
}

var (
	chatService *ChatService
	chatOnce    sync.Once
)

func GetChatService() *ChatService {
	chatOnce.Do(func() {
		sqlDB := conn.GetSQLDB()
		chatService = &ChatService{
			chatExecutor: chat.GetChatExecutor(),
			sqlDB:        sqlDB,
			queries:      db.New(sqlDB),
		}
	})
	return chatService
}

func fromNullRawMessage(nrm pqtype.NullRawMessage) []byte {
	if !nrm.Valid {
		return nil
	}
	return nrm.RawMessage
}

func (s *ChatService) CreateSession(ctx context.Context, dto *models.CreateSessionDto) (*models.ChatSessionDto, error) {
	if _, err := s.queries.GetAgentById(ctx, dto.AgentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, customerrors.ErrAgentNotFound
		}
		slog.ErrorContext(ctx, "failed to find agent for new session", "error", err, "agentId", dto.AgentID)
		return nil, err
	}

	defaultModel, err := s.queries.GetDefaultModel(ctx)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.ErrorContext(ctx, "failed to find default model for new session", "error", err)
			return nil, err
		}
	}

	if defaultModel.ID == uuid.Nil {
		defaultModel, err = s.queries.GetFirstModel(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "failed to find any model for new session", "error", err)
			return nil, err
		}
	}

	providerRow, err := s.queries.GetProviderById(ctx, defaultModel.ProviderID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find provider for default model", "error", err, "providerId", defaultModel.ProviderID)
		return nil, err
	}

	if providerRow.ApiKey == "" {
		return nil, customerrors.ErrProviderNotConfigured
	}

	sessionRow, err := s.queries.CreateSession(ctx, db.CreateSessionParams{
		AgentID:       dto.AgentID,
		LastUsedModel: defaultModel.ID,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to create chat session", "error", err)
		return nil, err
	}
	return models.ChatSessionRowToDto(sessionRow, nil), nil
}

func (s *ChatService) GetSession(ctx context.Context, sessionID uuid.UUID) (*models.ChatSessionDto, error) {
	session, err := s.queries.GetSessionById(ctx, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, customerrors.ErrSessionNotFound
		}
		slog.ErrorContext(ctx, "failed to find chat session", "error", err, "sessionId", sessionID)
		return nil, err
	}

	return s.loadSessionWithMessages(ctx, session)
}

func (s *ChatService) GetLastSession(ctx context.Context) (*models.ChatSessionDto, error) {
	session, err := s.queries.GetLastSession(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		slog.ErrorContext(ctx, "failed to find last chat session", "error", err)
		return nil, err
	}

	return s.loadSessionWithMessages(ctx, session)
}

func (s *ChatService) GetLastSessionByAgent(ctx context.Context, agentID uuid.UUID) (*models.ChatSessionDto, error) {
	session, err := s.queries.GetLastSessionByAgent(ctx, agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		slog.ErrorContext(ctx, "failed to find last chat session by agent", "error", err, "agentId", agentID)
		return nil, err
	}

	return s.loadSessionWithMessages(ctx, session)
}

func (s *ChatService) loadSessionWithMessages(ctx context.Context, session db.ChatSession) (*models.ChatSessionDto, error) {
	chatMessages, err := s.queries.GetMessagesBySession(ctx, session.ID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find chat messages", "error", err, "sessionId", session.ID)
		return nil, err
	}
	if len(chatMessages) == 0 {
		return models.ChatSessionRowToDto(session, nil), nil
	}

	modelIds, err := s.queries.GetDistinctModelIdsBySession(ctx, session.ID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get model IDs", "error", err, "sessionId", session.ID)
		return nil, err
	}

	modelMap := make(map[uuid.UUID]*models.ModelDto, len(modelIds))
	modelRows, err := s.queries.ListModelsByIds(ctx, modelIds)
	if err != nil {
		return nil, err
	}
	for _, mrow := range modelRows {
		modelMap[mrow.ID] = models.ModelRowToDto(mrow, nil)
	}

	messageDtos := lo.Map(chatMessages, func(cm db.ChatMessage, _ int) models.ChatMessageDto {
		return *models.ChatMessageRowToDto(cm, modelMap[cm.ModelID])
	})
	return models.ChatSessionRowToDto(session, messageDtos), nil
}

func (s *ChatService) ListSessions(ctx context.Context, request *pagination.PageRequest) (*pagination.PagedResponse[models.ChatSessionDto], error) {
	sessions, err := s.queries.ListSessions(ctx, db.ListSessionsParams{
		Limit:  int32(request.PageSize),
		Offset: int32((request.Page - 1) * request.PageSize),
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to list chat sessions", "error", err)
		return nil, err
	}

	total, err := s.queries.CountSessions(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to count sessions", "error", err)
		return nil, err
	}

	dtos := lo.Map(sessions, func(s db.ChatSession, _ int) models.ChatSessionDto {
		return *models.ChatSessionRowToDto(s, nil)
	})

	return &pagination.PagedResponse[models.ChatSessionDto]{
		Total:    total,
		PageSize: request.PageSize,
		Page:     request.Page,
		Data:     dtos,
	}, nil
}

func (s *ChatService) Chat(ctx context.Context, sessionID uuid.UUID, data *models.ChatDto) ([]*models.ChatMessageDto, error) {
	session, err := s.queries.GetSessionById(ctx, sessionID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find chat session", "error", err, "sessionId", sessionID)
		return nil, err
	}

	modelRow, err := s.queries.GetModelById(ctx, data.ModelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, customerrors.ErrModelNotFound
		}
		slog.ErrorContext(ctx, "failed to find model", "error", err, "modelId", data.ModelID)
		return nil, err
	}

	providerRow, err := s.queries.GetProviderById(ctx, modelRow.ProviderID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, customerrors.ErrProviderNotFound
		}
		slog.ErrorContext(ctx, "failed to find provider", "error", err, "providerId", modelRow.ProviderID)
		return nil, err
	}

	if providerRow.ApiKey == "" {
		return nil, customerrors.ErrProviderNotConfigured
	}

	agentRow, err := s.queries.GetAgentById(ctx, session.AgentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, customerrors.ErrAgentNotFound
		}
		slog.ErrorContext(ctx, "failed to find agent", "error", err, "agentId", session.AgentID)
		return nil, err
	}

	var systemPromptBuilder strings.Builder
	if err := consts.AgentBasePrompt.Execute(&systemPromptBuilder, map[string]any{
		"DateTime":  time.Now().Format(time.RFC3339),
		"AgentName": agentRow.Name,
		"AgentID":   agentRow.ID,
		"Soul":      agentRow.Soul,
	}); err != nil {
		slog.ErrorContext(ctx, "failed to build system prompt", "error", err)
		return nil, err
	}

	chatMessages, err := s.queries.GetMessagesBySession(ctx, session.ID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find chat messages", "error", err, "sessionId", sessionID)
		return nil, err
	}

	messages := lo.Map(chatMessages, func(cm db.ChatMessage, _ int) provider.Message {
		var toolCalls []models.ToolCall
		if raw := fromNullRawMessage(cm.ToolCalls); len(raw) > 0 {
			if err := json.Unmarshal(raw, &toolCalls); err != nil {
				slog.ErrorContext(ctx, "failed to unmarshal tool calls", "error", err, "sessionId", sessionID, "messageId", cm.ID)
			}
		}

		var toolResult *models.ToolResult
		if raw := fromNullRawMessage(cm.ToolResults); len(raw) > 0 {
			var tr models.ToolResult
			if err := json.Unmarshal(raw, &tr); err != nil {
				slog.ErrorContext(ctx, "failed to unmarshal tool result", "error", err, "sessionId", sessionID, "messageId", cm.ID)
			} else {
				toolResult = &tr
			}
		}

		msg := provider.Message{
			Role:       models.MessageRole(cm.Role),
			Content:    cm.Content,
			ToolCalls:  toolCalls,
			ToolResult: toolResult,
		}

		if data.ModelID == cm.ModelID {
			msg.ReasoningContent = cm.ReasoningContent
			if data.Thinking {
				if raw := fromNullRawMessage(cm.ProviderSpecifics); len(raw) > 0 {
					var ps models.ProviderSpecificData
					if err := json.Unmarshal(raw, &ps); err == nil {
						if len(ps.AnthropicThinkingBlocks) > 0 {
							msg.ReasoningBlocks = lo.Map(ps.AnthropicThinkingBlocks, func(b models.AnthropicThinkingBlock, _ int) provider.ReasoningBlock {
								if b.Type == "redacted_thinking" {
									return provider.ReasoningBlock{
										Signature: b.Data,
										Redacted:  true,
									}
								}
								return provider.ReasoningBlock{
									Summary:   b.Thinking,
									Signature: b.Signature,
								}
							})
						} else if len(ps.GeminiThinkingBlocks) > 0 {
							msg.ReasoningBlocks = lo.Map(ps.GeminiThinkingBlocks, func(b models.GeminiThinkingData, _ int) provider.ReasoningBlock {
								return provider.ReasoningBlock{
									Summary:   b.Summary,
									Signature: b.ThoughtSignature,
								}
							})
						} else if len(ps.OpenAIReasoningBlocks) > 0 {
							msg.ReasoningBlocks = lo.Map(ps.OpenAIReasoningBlocks, func(b models.OpenAIReasoningBlock, _ int) provider.ReasoningBlock {
								return provider.ReasoningBlock{
									Summary:   b.Summary,
									Signature: b.EncryptedContent,
								}
							})
						}
					}
				}
			}
		}

		return msg
	})
	messages = append([]provider.Message{{
		Role:    models.RoleSystem,
		Content: systemPromptBuilder.String(),
	}}, messages...)
	messages = append(messages, provider.Message{
		Role:    models.RoleUser,
		Content: data.Message,
	})

	if _, err := s.queries.CreateMessage(ctx, db.CreateMessageParams{
		SessionID:         session.ID,
		AgentID:           session.AgentID,
		Role:              string(models.RoleUser),
		Content:           data.Message,
		ToolCalls:         pqtype.NullRawMessage{Valid: false},
		ToolResults:       pqtype.NullRawMessage{Valid: false},
		ModelID:           modelRow.ID,
		ReasoningContent:  "",
		ProviderSpecifics: pqtype.NullRawMessage{Valid: false},
	}); err != nil {
		slog.ErrorContext(ctx, "failed to save user message", "error", err, "sessionId", sessionID)
		return nil, err
	}

	result, err := s.chatExecutor.Chat(ctx, &chat.ChatParams{
		BaseURL:                   providerRow.BaseUrl,
		APIKey:                    providerRow.ApiKey,
		Model:                     modelRow.Code,
		Messages:                  messages,
		AgentID:                   session.AgentID,
		APIType:                   models.APIType(providerRow.Type),
		Thinking:                  data.Thinking && modelRow.Thinking,
		ThinkingLevel:             data.ThinkingLevel,
		AnthropicAdaptiveThinking: modelRow.AnthropicAdaptiveThinking,
	})
	if err != nil {
		slog.ErrorContext(ctx, "chat completion failed", "error", err, "sessionId", sessionID)
		return nil, err
	}

	tx, err := s.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	qtx := s.queries.WithTx(tx)

	n := len(result.Messages)
	params := db.BatchCreateMessagesParams{
		SessionIds:           make([]uuid.UUID, n),
		AgentIds:             make([]uuid.UUID, n),
		Roles:                make([]string, n),
		Contents:             make([]string, n),
		ToolCallsArr:         make([]stdjson.RawMessage, n),
		ToolResultsArr:       make([]stdjson.RawMessage, n),
		ModelIds:             make([]uuid.UUID, n),
		ReasoningContents:    make([]string, n),
		ProviderSpecificsArr: make([]stdjson.RawMessage, n),
		CreatedAts:           make([]time.Time, n),
	}
	baseTime := time.Now()
	for i, m := range result.Messages {
		params.SessionIds[i] = session.ID
		params.AgentIds[i] = session.AgentID
		params.Roles[i] = string(models.MessageRole(m.Role))
		params.Contents[i] = m.Content
		params.ModelIds[i] = modelRow.ID
		params.ReasoningContents[i] = m.ReasoningContent
		params.CreatedAts[i] = baseTime.Add(time.Duration(i) * time.Millisecond)

		if len(m.ToolCalls) > 0 {
			if d, err := json.Marshal(m.ToolCalls); err != nil {
				slog.ErrorContext(ctx, "failed to marshal tool calls", "error", err, "sessionId", sessionID)
			} else {
				params.ToolCallsArr[i] = d
			}
		}

		if m.ToolResult != nil {
			if d, err := json.Marshal(m.ToolResult); err != nil {
				slog.ErrorContext(ctx, "failed to marshal tool result", "error", err, "sessionId", sessionID)
			} else {
				params.ToolResultsArr[i] = d
			}
		}

		if m.Role == models.RoleAssistant && len(m.ReasoningBlocks) > 0 {
			var specificData models.ProviderSpecificData
			switch models.APIType(providerRow.Type) {
			case models.APITypeAnthropic:
				specificData.AnthropicThinkingBlocks = lo.Map(m.ReasoningBlocks, func(b provider.ReasoningBlock, _ int) models.AnthropicThinkingBlock {
					if b.Redacted {
						return models.AnthropicThinkingBlock{
							Type: "redacted_thinking",
							Data: b.Signature,
						}
					}
					return models.AnthropicThinkingBlock{
						Type:      "thinking",
						Thinking:  b.Summary,
						Signature: b.Signature,
					}
				})
			case models.APITypeGemini:
				specificData.GeminiThinkingBlocks = lo.Map(m.ReasoningBlocks, func(b provider.ReasoningBlock, _ int) models.GeminiThinkingData {
					return models.GeminiThinkingData{
						ThoughtSignature: b.Signature,
						ThinkingLevel:    data.ThinkingLevel,
						Summary:          b.Summary,
					}
				})
			case models.APITypeOpenAI:
				specificData.OpenAIReasoningBlocks = lo.Map(m.ReasoningBlocks, func(b provider.ReasoningBlock, _ int) models.OpenAIReasoningBlock {
					return models.OpenAIReasoningBlock{
						Summary:          b.Summary,
						EncryptedContent: b.Signature,
					}
				})
			}
			if raw, err := json.Marshal(specificData); err == nil {
				params.ProviderSpecificsArr[i] = raw
			}
		}
	}

	msgRows, err := qtx.BatchCreateMessages(ctx, params)
	if err != nil {
		slog.ErrorContext(ctx, "failed to save messages", "error", err, "sessionId", sessionID)
		return nil, err
	}

	messageDtos := lo.Map(msgRows, func(r db.ChatMessage, _ int) *models.ChatMessageDto {
		return models.ChatMessageRowToDto(r, nil)
	})

	if err := qtx.UpdateSessionTokenAndModel(ctx, db.UpdateSessionTokenAndModelParams{
		ID:            session.ID,
		TokenConsumed: session.TokenConsumed + int64(result.TotalToken),
		LastUsedModel: modelRow.ID,
	}); err != nil {
		slog.ErrorContext(ctx, "failed to update token consumed", "error", err, "sessionId", sessionID)
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		slog.ErrorContext(ctx, "failed to commit chat messages", "error", err, "sessionId", sessionID)
		return nil, err
	}

	return messageDtos, nil
}
