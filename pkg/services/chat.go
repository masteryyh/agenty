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
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"github.com/masteryyh/agenty/pkg/utils/signal"
	"github.com/samber/lo"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ChatService struct {
	chatExecutor *chat.ChatExecutor
	db           *gorm.DB
	todosManager *tools.TodoManager
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
			todosManager: tools.GetTodoManager(),
		}
	})
	return chatService
}

func (s *ChatService) CreateSession(ctx context.Context, dto *models.CreateSessionDto) (*models.ChatSessionDto, error) {
	_, err := gorm.G[models.Agent](s.db).
		Where("id = ? AND deleted_at IS NULL", dto.AgentID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrAgentNotFound
		}
		slog.ErrorContext(ctx, "failed to find agent for new session", "error", err, "agentId", dto.AgentID)
		return nil, err
	}

	var defaultModel models.Model

	primaryEntry, primaryErr := gorm.G[models.AgentModel](s.db).
		Where("agent_id = ? AND sort_order = 0", dto.AgentID).
		First(ctx)
	if primaryErr == nil {
		m, err := gorm.G[models.Model](s.db).
			Where("id = ? AND deleted_at IS NULL", primaryEntry.ModelID).
			First(ctx)
		if err == nil {
			defaultModel = m
		}
	}

	if defaultModel.ID == uuid.Nil {
		m, err := gorm.G[models.Model](s.db).
			Where("default_model IS true AND deleted_at IS NULL").
			First(ctx)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			slog.ErrorContext(ctx, "failed to find default model for new session", "error", err)
			return nil, err
		}
		if err == nil {
			defaultModel = m
		}
	}

	if defaultModel.ID == uuid.Nil {
		m, err := gorm.G[models.Model](s.db).
			Where("deleted_at IS NULL").
			Order("created_at DESC").
			First(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "failed to find any model for new session", "error", err)
			return nil, err
		}
		defaultModel = m
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
		AgentID:       dto.AgentID,
		LastUsedModel: defaultModel.ID,
	}
	if err := gorm.G[models.ChatSession](s.db).Create(ctx, session); err != nil {
		slog.ErrorContext(ctx, "failed to create chat session", "error", err)
		return nil, err
	}

	go s.saveLastSessionAsMemory(dto.AgentID)

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

	return s.loadSessionWithMessages(ctx, &session)
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

	return s.loadSessionWithMessages(ctx, &session)
}

func (s *ChatService) GetLastSessionByAgent(ctx context.Context, agentID uuid.UUID) (*models.ChatSessionDto, error) {
	session, err := gorm.G[models.ChatSession](s.db).
		Where("agent_id = ? AND deleted_at IS NULL", agentID).
		Order("updated_at DESC").
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		slog.ErrorContext(ctx, "failed to find last chat session by agent", "error", err, "agentId", agentID)
		return nil, err
	}

	return s.loadSessionWithMessages(ctx, &session)
}

func (s *ChatService) loadSessionWithMessages(ctx context.Context, session *models.ChatSession) (*models.ChatSessionDto, error) {
	chatMessages, err := gorm.G[*models.ChatMessage](s.db).
		Where("session_id = ? AND deleted_at IS NULL", session.ID).
		Order("created_at ASC").
		Find(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find chat messages", "error", err, "sessionId", session.ID)
		return nil, err
	}
	if len(chatMessages) == 0 {
		dto := session.ToDto(nil)
		if todos := s.todosManager.List(session.ID); len(todos) > 0 {
			dto.Todos = todos
		}
		return dto, nil
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
	dto := session.ToDto(messageDtos)
	if todos := s.todosManager.List(session.ID); len(todos) > 0 {
		dto.Todos = todos
	}
	return dto, nil
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
	res, err := s.loadChatResources(ctx, sessionID, data.ModelID)
	if err != nil {
		return nil, err
	}

	systemPrompt, err := buildSystemPrompt(&res.agent)
	if err != nil {
		slog.ErrorContext(ctx, "failed to build system prompt", "error", err)
		return nil, err
	}

	messages, err := s.loadHistoryMessages(ctx, res.session.ID, res.model.ID, data.Thinking && res.model.Thinking)
	if err != nil {
		return nil, err
	}
	messages = append([]provider.Message{{Role: models.RoleSystem, Content: systemPrompt}}, messages...)
	messages = append(messages, provider.Message{Role: models.RoleUser, Content: data.Message})

	if err := s.saveUserMessage(ctx, &res.session, data.Message); err != nil {
		return nil, err
	}

	result, err := s.chatExecutor.Chat(ctx, buildChatParams(res, messages, data))
	if err != nil {
		slog.ErrorContext(ctx, "chat completion failed", "error", err, "sessionId", sessionID)
		return nil, err
	}

	baseTime := time.Now()
	newMessages := make([]models.ChatMessage, 0, len(result.Messages))
	for i, m := range result.Messages {
		newMessages = append(newMessages, buildChatMessage(m, res.session.ID, res.session.AgentID, res.model.ID, res.chatProvider.Type, baseTime.Add(time.Duration(i)*time.Millisecond), data.ThinkingLevel))
	}

	if err := s.saveMessagesAndUpdateSession(ctx, &res.session, res.model.ID, newMessages, result.TotalToken); err != nil {
		slog.ErrorContext(ctx, "failed to save chat messages and update session", "error", err, "sessionId", sessionID)
		return nil, err
	}

	messageDtos := lo.Map(newMessages, func(m models.ChatMessage, _ int) *models.ChatMessageDto {
		return m.ToDto(nil)
	})
	return messageDtos, nil
}

func (s *ChatService) StreamChat(ctx context.Context, sessionID uuid.UUID, data *models.ChatDto) (<-chan provider.StreamEvent, error) {
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

	candidates, err := s.resolveModelList(ctx, session.AgentID, data.ModelID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to resolve models", "error", err, "sessionId", sessionID)
		return nil, err
	}

	agent, err := gorm.G[models.Agent](s.db).
		Where("id = ? AND deleted_at IS NULL", session.AgentID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrAgentNotFound
		}
		slog.ErrorContext(ctx, "failed to find agent", "error", err, "agentId", session.AgentID)
		return nil, err
	}

	systemPrompt, err := buildSystemPrompt(&agent)
	if err != nil {
		slog.ErrorContext(ctx, "failed to build system prompt", "error", err)
		return nil, err
	}

	primary := candidates[0]
	baseMessages, err := s.loadHistoryMessages(ctx, session.ID, primary.model.ID, data.Thinking && primary.model.Thinking)
	if err != nil {
		return nil, err
	}
	baseMessages = append([]provider.Message{{Role: models.RoleSystem, Content: systemPrompt}}, baseMessages...)
	baseMessages = append(baseMessages, provider.Message{Role: models.RoleUser, Content: data.Message})

	if err := s.saveUserMessage(ctx, &session, data.Message); err != nil {
		return nil, err
	}

	out := make(chan provider.StreamEvent, 64)

	safe.GoOnce("chat-service-stream", func() {
		defer close(out)

		var collectedMessages []provider.Message
		var totalTokens int64

		for candidateIdx, candidate := range candidates {
			isFallback := candidateIdx > 0 || (data.ModelID != uuid.Nil && candidate.model.ID != data.ModelID)

			if isFallback {
				var thinkingLevels []string
				if len(candidate.model.ThinkingLevels) > 0 {
					_ = json.Unmarshal(candidate.model.ThinkingLevels, &thinkingLevels)
				}
				select {
				case out <- provider.StreamEvent{
					Type:                provider.EventModelSwitch,
					ModelID:             candidate.model.ID.String(),
					ModelName:           candidate.chatProvider.Name + "/" + candidate.model.Name,
					ModelThinking:       candidate.model.Thinking,
					ModelThinkingLevels: thinkingLevels,
				}:
				case <-ctx.Done():
					return
				}
			}

			msgs := baseMessages
			if !candidate.model.Thinking {
				msgs = stripThinkingData(baseMessages)
			}

			res := &chatResources{
				session:      session,
				model:        candidate.model,
				chatProvider: candidate.chatProvider,
				agent:        agent,
			}

			executorCh, err := s.chatExecutor.StreamChat(ctx, buildChatParams(res, msgs, data))
			if err != nil {
				slog.WarnContext(ctx, "model unavailable, trying next", "model", candidate.model.Name, "error", err)
				continue
			}

			var apiErr string
			for evt := range executorCh {
				if evt.Type == provider.EventError {
					apiErr = evt.Error
					slog.WarnContext(ctx, "model returned error, trying next", "model", candidate.model.Name, "error", evt.Error)
					go func() {
						for range executorCh {
						}
					}()
					break
				}

				if evt.Type == provider.EventMessageDone && evt.Message != nil {
					collectedMessages = append(collectedMessages, *evt.Message)
				}
				if evt.Type == provider.EventToolResult && evt.ToolResult != nil {
					collectedMessages = append(collectedMessages, provider.Message{
						Role:       models.RoleTool,
						Content:    evt.ToolResult.Content,
						ToolResult: evt.ToolResult,
					})
				}
				if evt.Type == provider.EventUsage && evt.Usage != nil {
					totalTokens = evt.Usage.TotalTokens
				}

				if evt.Type == provider.EventDone {
					s.persistStreamMessages(signal.GetBaseContext(), res, collectedMessages, totalTokens, data.ThinkingLevel)
					select {
					case out <- evt:
					case <-ctx.Done():
					}
					return
				}

				select {
				case out <- evt:
				case <-ctx.Done():
					s.persistStreamMessages(signal.GetBaseContext(), res, collectedMessages, totalTokens, data.ThinkingLevel)
					return
				}
			}

			if apiErr == "" {
				return
			}

			collectedMessages = nil
			totalTokens = 0
		}

		select {
		case out <- provider.StreamEvent{
			Type:  provider.EventError,
			Error: "all configured models are unavailable, please check your model configuration",
		}:
		case <-ctx.Done():
		}
	})

	return out, nil
}

func (s *ChatService) persistStreamMessages(ctx context.Context, res *chatResources, collectedMessages []provider.Message, totalTokens int64, thinkingLevel string) {
	baseTime := time.Now()
	newMessages := make([]models.ChatMessage, 0, len(collectedMessages))
	for i, m := range collectedMessages {
		newMessages = append(newMessages, buildChatMessage(m, res.session.ID, res.session.AgentID, res.model.ID, res.chatProvider.Type, baseTime.Add(time.Duration(i)*time.Millisecond), thinkingLevel))
	}

	if len(newMessages) == 0 {
		return
	}

	if err := s.saveMessagesAndUpdateSession(ctx, &res.session, res.model.ID, newMessages, totalTokens); err != nil {
		slog.ErrorContext(ctx, "failed to persist stream messages", "error", err, "sessionId", res.session.ID)
	}
}

type chatResources struct {
	session           models.ChatSession
	model             models.Model
	chatProvider      models.ModelProvider
	agent             models.Agent
	fallbackModelName string
}

type modelCandidate struct {
	model        models.Model
	chatProvider models.ModelProvider
}

func (s *ChatService) resolveModelList(ctx context.Context, agentID, preferredModelID uuid.UUID) ([]modelCandidate, error) {
	tryModel := func(mid uuid.UUID) (modelCandidate, bool, error) {
		mdl, err := gorm.G[models.Model](s.db).
			Where("id = ? AND deleted_at IS NULL", mid).
			First(ctx)
		if err != nil {
			return modelCandidate{}, false, err
		}
		prov, err := gorm.G[models.ModelProvider](s.db).
			Where("id = ? AND deleted_at IS NULL", mdl.ProviderID).
			First(ctx)
		if err != nil {
			return modelCandidate{}, false, err
		}
		if prov.APIKey == "" {
			return modelCandidate{}, false, customerrors.ErrProviderNotConfigured
		}
		return modelCandidate{model: mdl, chatProvider: prov}, true, nil
	}

	var result []modelCandidate
	seen := map[uuid.UUID]bool{}

	if preferredModelID != uuid.Nil {
		if c, ok, err := tryModel(preferredModelID); ok {
			result = append(result, c)
			seen[preferredModelID] = true
		} else if err != nil {
			return nil, err
		}
	}

	agentModels, err := gorm.G[models.AgentModel](s.db).
		Where("agent_id = ?", agentID).
		Order("sort_order ASC").
		Find(ctx)
	if err == nil {
		for _, am := range agentModels {
			if seen[am.ModelID] {
				continue
			}
			if c, ok, err := tryModel(am.ModelID); ok {
				result = append(result, c)
				seen[am.ModelID] = true
			} else if err != nil {
				return nil, err
			}
		}
	}

	if len(result) == 0 {
		return nil, customerrors.ErrModelNotFound
	}
	return result, nil
}

func stripThinkingData(messages []provider.Message) []provider.Message {
	result := make([]provider.Message, len(messages))
	for i, m := range messages {
		m.ReasoningContent = ""
		m.ReasoningBlocks = nil
		result[i] = m
	}
	return result
}

func (s *ChatService) resolveModel(ctx context.Context, agentID, preferredModelID uuid.UUID) (models.Model, models.ModelProvider, error) {
	tryModel := func(mid uuid.UUID) (models.Model, models.ModelProvider, bool) {
		mdl, err := gorm.G[models.Model](s.db).
			Where("id = ? AND deleted_at IS NULL", mid).
			First(ctx)
		if err != nil {
			return models.Model{}, models.ModelProvider{}, false
		}
		prov, err := gorm.G[models.ModelProvider](s.db).
			Where("id = ? AND deleted_at IS NULL", mdl.ProviderID).
			First(ctx)
		if err != nil || prov.APIKey == "" {
			return models.Model{}, models.ModelProvider{}, false
		}
		return mdl, prov, true
	}

	if preferredModelID != uuid.Nil {
		if mdl, prov, ok := tryModel(preferredModelID); ok {
			return mdl, prov, nil
		}
		slog.WarnContext(ctx, "preferred model unavailable, falling back to agent model list", "modelId", preferredModelID)
	}

	agentModels, err := gorm.G[models.AgentModel](s.db).
		Where("agent_id = ?", agentID).
		Order("sort_order ASC").
		Find(ctx)
	if err != nil || len(agentModels) == 0 {
		return models.Model{}, models.ModelProvider{}, customerrors.ErrModelNotFound
	}

	for _, am := range agentModels {
		if am.ModelID == preferredModelID {
			continue
		}
		if mdl, prov, ok := tryModel(am.ModelID); ok {
			return mdl, prov, nil
		}
	}

	return models.Model{}, models.ModelProvider{}, customerrors.ErrProviderNotConfigured
}

func (s *ChatService) loadChatResources(ctx context.Context, sessionID, modelID uuid.UUID) (*chatResources, error) {
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

	model, chatProvider, err := s.resolveModel(ctx, session.AgentID, modelID)
	if err != nil {
		if customerrors.GetBusinessError(err) == nil {
			slog.ErrorContext(ctx, "failed to resolve model", "error", err, "sessionId", sessionID, "preferredModelId", modelID)
			return nil, fmt.Errorf("failed to resolve model: %w", err)
		}
		return nil, err
	}

	agent, err := gorm.G[models.Agent](s.db).
		Where("id = ? AND deleted_at IS NULL", session.AgentID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, customerrors.ErrAgentNotFound
		}
		slog.ErrorContext(ctx, "failed to find agent", "error", err, "agentId", session.AgentID)
		return nil, err
	}

	res := &chatResources{
		session:      session,
		model:        model,
		chatProvider: chatProvider,
		agent:        agent,
	}

	if model.ID != modelID {
		res.fallbackModelName = chatProvider.Name + "/" + model.Name
	}

	return res, nil
}

func buildSystemPrompt(agent *models.Agent) (string, error) {
	var sb strings.Builder
	if err := consts.AgentBasePrompt.Execute(&sb, map[string]any{
		"DateTime":  time.Now().Format(time.RFC3339),
		"AgentName": agent.Name,
		"AgentID":   agent.ID,
		"Soul":      agent.Soul,
	}); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func (s *ChatService) loadHistoryMessages(ctx context.Context, sessionID, modelID uuid.UUID, thinking bool) ([]provider.Message, error) {
	chatMessages, err := gorm.G[*models.ChatMessage](s.db).
		Where("session_id = ? AND deleted_at IS NULL", sessionID).
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

		if modelID == cm.ModelID {
			msg.ReasoningContent = cm.ReasoningContent
			if thinking && len(cm.ProviderSpecifics) > 0 {
				var ps models.ProviderSpecificData
				if err := json.Unmarshal(cm.ProviderSpecifics, &ps); err == nil {
					if len(ps.AnthropicThinkingBlocks) > 0 {
						msg.ReasoningBlocks = lo.Map(ps.AnthropicThinkingBlocks, func(b models.AnthropicThinkingBlock, _ int) provider.ReasoningBlock {
							if b.Type == "redacted_thinking" {
								return provider.ReasoningBlock{Signature: b.Data, Redacted: true}
							}
							return provider.ReasoningBlock{Summary: b.Thinking, Signature: b.Signature}
						})
					} else if len(ps.GeminiThinkingBlocks) > 0 {
						msg.ReasoningBlocks = lo.Map(ps.GeminiThinkingBlocks, func(b models.GeminiThinkingData, _ int) provider.ReasoningBlock {
							return provider.ReasoningBlock{Summary: b.Summary, Signature: b.ThoughtSignature}
						})
					} else if len(ps.OpenAIReasoningBlocks) > 0 {
						msg.ReasoningBlocks = lo.Map(ps.OpenAIReasoningBlocks, func(b models.OpenAIReasoningBlock, _ int) provider.ReasoningBlock {
							return provider.ReasoningBlock{Summary: b.Summary, Signature: b.EncryptedContent}
						})
					}
				}
			}
		}

		return msg
	})
	return messages, nil
}

func (s *ChatService) saveUserMessage(ctx context.Context, session *models.ChatSession, content string) error {
	msg := models.ChatMessage{
		SessionID: session.ID,
		AgentID:   session.AgentID,
		Role:      models.RoleUser,
		Content:   content,
	}
	if err := gorm.G[models.ChatMessage](s.db).Create(ctx, &msg); err != nil {
		slog.ErrorContext(ctx, "failed to save user message", "error", err, "sessionId", session.ID)
		return err
	}
	return nil
}

func buildChatParams(res *chatResources, messages []provider.Message, data *models.ChatDto) *chat.ChatParams {
	return &chat.ChatParams{
		BaseURL:                   res.chatProvider.BaseURL,
		APIKey:                    res.chatProvider.APIKey,
		Model:                     res.model.Code,
		Messages:                  messages,
		AgentID:                   res.session.AgentID,
		SessionID:                 res.session.ID,
		ModelID:                   res.model.ID,
		APIType:                   res.chatProvider.Type,
		Thinking:                  data.Thinking && res.model.Thinking,
		ThinkingLevel:             data.ThinkingLevel,
		AnthropicAdaptiveThinking: res.model.AnthropicAdaptiveThinking,
	}
}

func buildChatMessage(m provider.Message, sessionID, agentID, modelID uuid.UUID, providerType models.APIType, timestamp time.Time, thinkingLevel string) models.ChatMessage {
	var rawCalls []byte
	if len(m.ToolCalls) > 0 {
		if d, err := json.Marshal(m.ToolCalls); err != nil {
			slog.Error("failed to marshal tool calls", "error", err, "sessionId", sessionID)
		} else {
			rawCalls = d
		}
	}

	var rawCallResult []byte
	if m.ToolResult != nil {
		if d, err := json.Marshal(m.ToolResult); err != nil {
			slog.Error("failed to marshal tool result", "error", err, "sessionId", sessionID)
		} else {
			rawCallResult = d
		}
	}

	chatMsg := models.ChatMessage{
		SessionID:        sessionID,
		AgentID:          agentID,
		Role:             models.MessageRole(m.Role),
		Content:          m.Content,
		ToolCalls:        datatypes.JSON(rawCalls),
		ToolResults:      datatypes.JSON(rawCallResult),
		ModelID:          modelID,
		ReasoningContent: m.ReasoningContent,
		CreatedAt:        timestamp,
	}

	if m.Role == models.RoleAssistant && len(m.ReasoningBlocks) > 0 {
		var specificData models.ProviderSpecificData
		switch providerType {
		case models.APITypeAnthropic:
			specificData.AnthropicThinkingBlocks = lo.Map(m.ReasoningBlocks, func(b provider.ReasoningBlock, _ int) models.AnthropicThinkingBlock {
				if b.Redacted {
					return models.AnthropicThinkingBlock{Type: "redacted_thinking", Data: b.Signature}
				}
				return models.AnthropicThinkingBlock{Type: "thinking", Thinking: b.Summary, Signature: b.Signature}
			})
		case models.APITypeGemini:
			specificData.GeminiThinkingBlocks = lo.Map(m.ReasoningBlocks, func(b provider.ReasoningBlock, _ int) models.GeminiThinkingData {
				return models.GeminiThinkingData{ThoughtSignature: b.Signature, ThinkingLevel: thinkingLevel, Summary: b.Summary}
			})
		case models.APITypeOpenAI:
			specificData.OpenAIReasoningBlocks = lo.Map(m.ReasoningBlocks, func(b provider.ReasoningBlock, _ int) models.OpenAIReasoningBlock {
				return models.OpenAIReasoningBlock{Summary: b.Summary, EncryptedContent: b.Signature}
			})
		}
		if raw, err := json.Marshal(specificData); err == nil {
			chatMsg.ProviderSpecifics = datatypes.JSON(raw)
		}
	}

	return chatMsg
}

func (s *ChatService) saveMessagesAndUpdateSession(ctx context.Context, session *models.ChatSession, modelID uuid.UUID, messages []models.ChatMessage, totalTokens int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&messages).Error; err != nil {
			return fmt.Errorf("failed to save assistant messages: %w", err)
		}

		session.TokenConsumed += totalTokens
		session.LastUsedModel = modelID
		if err := tx.Model(&models.ChatSession{}).
			Where("id = ?", session.ID).
			Updates(map[string]any{
				"token_consumed":  session.TokenConsumed,
				"last_used_model": session.LastUsedModel,
			}).Error; err != nil {
			return fmt.Errorf("failed to update chat session: %w", err)
		}
		return nil
	})
}

func (s *ChatService) saveLastSessionAsMemory(agentID uuid.UUID) {
	ctx := signal.GetBaseContext()

	prevSession, err := gorm.G[models.ChatSession](s.db).
		Where("agent_id = ? AND deleted_at IS NULL", agentID).
		Order("created_at DESC").
		Offset(1).
		First(ctx)
	if err != nil {
		return
	}

	_, err = gorm.G[models.KnowledgeItem](s.db).
		Where("source_session_id = ? AND deleted_at IS NULL", prevSession.ID).
		First(ctx)
	if err == nil {
		return
	}

	messages, err := gorm.G[*models.ChatMessage](s.db).
		Where("session_id = ? AND deleted_at IS NULL", prevSession.ID).
		Order("created_at ASC").
		Find(ctx)
	if err != nil || len(messages) == 0 {
		return
	}

	var sb strings.Builder
	for _, msg := range messages {
		if msg.Role == models.RoleTool || msg.Role == models.RoleSystem {
			continue
		}
		content := msg.Content
		if content == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", msg.Role, content))
	}

	sessionText := sb.String()
	if strings.TrimSpace(sessionText) == "" {
		return
	}

	title := fmt.Sprintf("Session %s", prevSession.ID.String()[:8])
	if firstUserMsg := findFirstUserMessage(messages); firstUserMsg != "" {
		if len(firstUserMsg) > 100 {
			firstUserMsg = firstUserMsg[:97] + "..."
		}
		title = firstUserMsg
	}

	_, err = GetKnowledgeService().CreateSessionMemory(ctx, agentID, prevSession.ID, title, sessionText)
	if err != nil {
		slog.ErrorContext(ctx, "failed to save session as memory", "sessionId", prevSession.ID, "error", err)
	}
}

func findFirstUserMessage(messages []*models.ChatMessage) string {
	for _, msg := range messages {
		if msg.Role == models.RoleUser && strings.TrimSpace(msg.Content) != "" {
			return msg.Content
		}
	}
	return ""
}
