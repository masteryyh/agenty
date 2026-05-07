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
	"html"
	"log/slog"
	"strings"
	"sync"
	"time"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/chat"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/providers"
	"github.com/masteryyh/agenty/pkg/tools"
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

const skillCountThreshold = 30
const relevantMemoryLimit = 6

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

	if err := chat.RunSessionHooks(ctx, chat.SessionHookAfterSessionCreated, &chat.SessionHookContext{
		SessionID: session.ID,
		AgentID:   session.AgentID,
		Session:   session,
	}); err != nil {
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

func (s *ChatService) SetSessionCwd(ctx context.Context, sessionID uuid.UUID, cwd *string, agentsMD *string) error {
	session, err := gorm.G[models.ChatSession](s.db).
		Where("id = ? AND deleted_at IS NULL", sessionID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return customerrors.ErrSessionNotFound
		}
		slog.ErrorContext(ctx, "failed to find chat session", "error", err, "sessionId", sessionID)
		return err
	}

	skillSvc := GetSkillService()

	if session.Cwd != nil {
		if err := skillSvc.DropSessionTable(ctx, sessionID); err != nil {
			slog.WarnContext(ctx, "failed to drop old session skills table", "sessionId", sessionID, "error", err)
		}
	}

	updates := map[string]any{"cwd": cwd, "agents_md": agentsMD}
	if err := s.db.WithContext(ctx).
		Model(&models.ChatSession{}).
		Where("id = ?", session.ID).
		Updates(updates).Error; err != nil {
		slog.ErrorContext(ctx, "failed to update session cwd", "error", err, "sessionId", sessionID)
		return err
	}

	if cwd != nil {
		if err := skillSvc.CreateSessionTable(ctx, sessionID); err != nil {
			slog.WarnContext(ctx, "failed to create session skills table", "sessionId", sessionID, "error", err)
		} else {
			if err := skillSvc.PopulateSessionSkills(ctx, sessionID, *cwd); err != nil {
				slog.WarnContext(ctx, "failed to populate session skills", "sessionId", sessionID, "error", err)
			}
		}
	}

	return nil
}

func (s *ChatService) loadChatSession(ctx context.Context, sessionID uuid.UUID) (models.ChatSession, error) {
	session, err := gorm.G[models.ChatSession](s.db).
		Where("id = ? AND deleted_at IS NULL", sessionID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.ChatSession{}, customerrors.ErrSessionNotFound
		}
		slog.ErrorContext(ctx, "failed to find chat session", "error", err, "sessionId", sessionID)
		return models.ChatSession{}, err
	}
	return session, nil
}

func sessionCwd(session models.ChatSession) string {
	if session.Cwd == nil {
		return ""
	}
	return *session.Cwd
}

func streamError(err error) <-chan providers.StreamEvent {
	out := make(chan providers.StreamEvent, 1)
	out <- providers.StreamEvent{Type: providers.EventError, Error: err.Error()}
	close(out)
	return out
}

func (s *ChatService) Chat(ctx context.Context, sessionID uuid.UUID, data *models.ChatDto) ([]*models.ChatMessageDto, error) {
	session, err := s.loadChatSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if err := chat.RunSessionHooks(ctx, chat.SessionHookAfterUserInput, &chat.SessionHookContext{
		SessionID: session.ID,
		AgentID:   session.AgentID,
		ModelID:   data.ModelID,
		Cwd:       sessionCwd(session),
		Input:     data,
	}); err != nil {
		return nil, err
	}

	res, err := s.loadChatResources(ctx, sessionID, data.ModelID)
	if err != nil {
		return nil, err
	}

	messages, err := s.loadHistoryMessages(ctx, res.session.ID, res.model.ID, data.Thinking && res.model.Thinking)
	if err != nil {
		return nil, err
	}

	skillsXML := s.resolveSkillsXML(ctx, &res.session, data.Message, messages)
	memoriesXML := s.resolveMemoriesXML(ctx, &res.session, data.Message)
	todosXML := s.resolveTodosXML(res.session.ID)

	systemPrompt, err := buildSystemPrompt(&res.agent, &res.session, skillsXML, memoriesXML, todosXML)
	if err != nil {
		slog.ErrorContext(ctx, "failed to build system prompt", "error", err)
		return nil, err
	}

	messages = append([]providers.Message{{Role: models.RoleSystem, Content: systemPrompt}}, messages...)
	messages = append(messages, providers.Message{Role: models.RoleUser, Content: data.Message})

	roundID, err := s.saveUserMessage(ctx, &res.session, data.Message)
	if err != nil {
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
		newMessages = append(newMessages, buildChatMessage(m, res.session.ID, roundID, res.session.AgentID, res.model.ID, res.chatProvider.Type, baseTime.Add(time.Duration(i)*time.Millisecond), data.ThinkingLevel))
	}

	if err := s.saveMessages(ctx, &res.session, res.model.ID, roundID, newMessages, result.TotalToken); err != nil {
		slog.ErrorContext(ctx, "failed to save chat messages and update session", "error", err, "sessionId", sessionID)
		return nil, err
	}

	messageDtos := lo.Map(newMessages, func(m models.ChatMessage, _ int) *models.ChatMessageDto {
		return m.ToDto(nil)
	})
	return messageDtos, nil
}

func (s *ChatService) StreamChat(ctx context.Context, sessionID uuid.UUID, data *models.ChatDto) (<-chan providers.StreamEvent, error) {
	session, err := s.loadChatSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if err := chat.RunSessionHooks(ctx, chat.SessionHookAfterUserInput, &chat.SessionHookContext{
		SessionID: session.ID,
		AgentID:   session.AgentID,
		ModelID:   data.ModelID,
		Cwd:       sessionCwd(session),
		Input:     data,
	}); err != nil {
		return streamError(err), nil
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

	primary := candidates[0]
	baseMessages, err := s.loadHistoryMessages(ctx, session.ID, primary.model.ID, data.Thinking && primary.model.Thinking)
	if err != nil {
		return nil, err
	}

	skillsXML := s.resolveSkillsXML(ctx, &session, data.Message, baseMessages)
	memoriesXML := s.resolveMemoriesXML(ctx, &session, data.Message)
	todosXML := s.resolveTodosXML(session.ID)

	systemPrompt, err := buildSystemPrompt(&agent, &session, skillsXML, memoriesXML, todosXML)
	if err != nil {
		slog.ErrorContext(ctx, "failed to build system prompt", "error", err)
		return nil, err
	}

	baseMessages = append([]providers.Message{{Role: models.RoleSystem, Content: systemPrompt}}, baseMessages...)
	baseMessages = append(baseMessages, providers.Message{Role: models.RoleUser, Content: data.Message})

	roundID, err := s.saveUserMessage(ctx, &session, data.Message)
	if err != nil {
		return nil, err
	}

	out := make(chan providers.StreamEvent, 64)

	safe.GoOnce("chat-service-stream", func() {
		defer close(out)

		var collectedMessages []providers.Message
		var totalTokens int64

		for candidateIdx, candidate := range candidates {
			isFallback := candidateIdx > 0 || (data.ModelID != uuid.Nil && candidate.model.ID != data.ModelID)

			if isFallback {
				var thinkingLevels []string
				if len(candidate.model.ThinkingLevels) > 0 {
					_ = json.Unmarshal(candidate.model.ThinkingLevels, &thinkingLevels)
				}
				select {
				case out <- providers.StreamEvent{
					Type:                providers.EventModelSwitch,
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
				if evt.Type == providers.EventError {
					apiErr = evt.Error
					slog.WarnContext(ctx, "model returned error, trying next", "model", candidate.model.Name, "error", evt.Error)
					go func() {
						for range executorCh {
						}
					}()
					break
				}

				if evt.Type == providers.EventMessageDone && evt.Message != nil {
					collectedMessages = append(collectedMessages, *evt.Message)
				}
				if evt.Type == providers.EventToolResult && evt.ToolResult != nil {
					collectedMessages = append(collectedMessages, providers.Message{
						Role:       models.RoleTool,
						Content:    evt.ToolResult.Content,
						ToolResult: evt.ToolResult,
					})
				}
				if evt.Type == providers.EventUsage && evt.Usage != nil {
					totalTokens = evt.Usage.TotalTokens
				}

				if evt.Type == providers.EventDone {
					s.persistStreamMessages(signal.GetBaseContext(), res, roundID, collectedMessages, totalTokens, data.ThinkingLevel)
					select {
					case out <- evt:
					case <-ctx.Done():
					}
					return
				}

				select {
				case out <- evt:
				case <-ctx.Done():
					s.persistStreamMessages(signal.GetBaseContext(), res, roundID, collectedMessages, totalTokens, data.ThinkingLevel)
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
		case out <- providers.StreamEvent{
			Type:  providers.EventError,
			Error: "all configured models are unavailable, please check your model configuration",
		}:
		case <-ctx.Done():
		}
	})

	return out, nil
}

func (s *ChatService) persistStreamMessages(ctx context.Context, res *chatResources, roundID uuid.UUID, collectedMessages []providers.Message, totalTokens int64, thinkingLevel string) {
	baseTime := time.Now()
	newMessages := make([]models.ChatMessage, 0, len(collectedMessages))
	for i, m := range collectedMessages {
		newMessages = append(newMessages, buildChatMessage(m, res.session.ID, roundID, res.session.AgentID, res.model.ID, res.chatProvider.Type, baseTime.Add(time.Duration(i)*time.Millisecond), thinkingLevel))
	}

	if len(newMessages) == 0 {
		return
	}

	if err := s.saveMessages(ctx, &res.session, res.model.ID, roundID, newMessages, totalTokens); err != nil {
		slog.ErrorContext(ctx, "failed to persist stream messages", "error", err, "sessionId", res.session.ID)
	}
}

func formatSkillsXML(skills []models.SkillDto) string {
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, skill := range skills {
		fmt.Fprintf(&sb, "<skill>\n    <name>%s</name>\n    <description>%s</description>\n    <path>%s</path>\n</skill>\n", skill.Name, skill.Description, skill.SkillMDPath)
	}
	return sb.String()
}

func formatSearchResultsXML(skills []models.SkillSearchResult) string {
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, skill := range skills {
		fmt.Fprintf(&sb, "<skill>\n    <name>%s</name>\n    <description>%s</description>\n    <path>%s</path>\n</skill>\n", skill.Name, skill.Description, skill.SkillMDPath)
	}
	return sb.String()
}

func formatMemoriesXML(memories []models.KBSearchResult) string {
	if len(memories) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, memory := range memories {
		title := memory.ItemTitle
		if title == "" {
			title = memory.ItemID.String()
		}
		content := strings.TrimSpace(memory.Content)
		if len(content) > 1200 {
			content = content[:1197] + "..."
		}
		fmt.Fprintf(&sb, "<memory>\n    <category>%s</category>\n    <title>%s</title>\n    <chunkIndex>%d</chunkIndex>\n    <content>%s</content>\n</memory>\n", html.EscapeString(string(memory.Category)), html.EscapeString(title), memory.ChunkIndex, html.EscapeString(content))
	}
	return sb.String()
}

func formatTodosXML(todos []models.TodoItemDto) string {
	if len(todos) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, todo := range todos {
		fmt.Fprintf(&sb, "<todo id=\"%d\" status=\"%s\">%s</todo>\n", todo.ID, html.EscapeString(todo.Status), html.EscapeString(todo.Content))
	}
	return sb.String()
}

func (s *ChatService) resolveSkillsXML(ctx context.Context, session *models.ChatSession, userMessage string, historyMessages []providers.Message) string {
	skillSvc := GetSkillService()

	count, err := skillSvc.CountSessionSkills(ctx, session.ID)
	if err != nil {
		slog.WarnContext(ctx, "failed to count session skills", "error", err, "sessionId", session.ID)
		return ""
	}

	if count == 0 {
		return ""
	}

	if count <= skillCountThreshold {
		skills, err := skillSvc.ListSessionSkillSummaries(ctx, session.ID)
		if err != nil {
			slog.WarnContext(ctx, "failed to list session skill summaries", "error", err, "sessionId", session.ID)
			return ""
		}
		return formatSkillsXML(skills)
	}

	results := s.preSelectSkills(ctx, session, userMessage, historyMessages)
	return formatSearchResultsXML(results)
}

func (s *ChatService) resolveMemoriesXML(ctx context.Context, session *models.ChatSession, userMessage string) string {
	if strings.TrimSpace(userMessage) == "" {
		return ""
	}

	memories, err := GetKnowledgeService().HybridSearch(ctx, session.AgentID, userMessage, relevantMemoryLimit)
	if err != nil {
		slog.WarnContext(ctx, "failed to resolve relevant memories", "error", err, "sessionId", session.ID)
	}
	return formatMemoriesXML(memories)
}

func (s *ChatService) resolveTodosXML(sessionID uuid.UUID) string {
	return formatTodosXML(s.todosManager.List(sessionID))
}

func (s *ChatService) preSelectSkills(ctx context.Context, session *models.ChatSession, userMessage string, historyMessages []providers.Message) []models.SkillSearchResult {
	lightModel, lightProvider, err := s.resolveLightModel(ctx)
	if err != nil {
		slog.WarnContext(ctx, "no light model available for skill pre-selection, falling back to find_skill", "error", err)
		return nil
	}

	promptContext := buildSkillSelectionContext(session, userMessage, historyMessages)
	selectionPrompt := fmt.Sprintf(consts.SkillSelectionPrompt, promptContext)

	result, err := s.chatExecutor.Chat(ctx, &chat.ChatParams{
		Messages: []providers.Message{
			{Role: models.RoleSystem, Content: selectionPrompt},
			{Role: models.RoleUser, Content: "Generate search keywords for finding relevant skills."},
		},
		Model:     lightModel.Code,
		AgentID:   session.AgentID,
		SessionID: session.ID,
		ModelID:   lightModel.ID,
		BaseURL:   lightProvider.BaseURL,
		APIKey:    lightProvider.APIKey,
		APIType:   lightProvider.Type,
	})
	if err != nil {
		slog.WarnContext(ctx, "skill pre-selection LLM call failed", "error", err)
		return nil
	}

	if len(result.Messages) == 0 {
		return nil
	}

	lastContent := result.Messages[len(result.Messages)-1].Content
	keywords := parseKeywordLines(lastContent)
	if len(keywords) == 0 {
		return nil
	}

	skillSvc := GetSkillService()
	seen := make(map[string]bool)
	var allResults []models.SkillSearchResult

	for _, kw := range keywords {
		results, err := skillSvc.SearchSkills(ctx, &session.ID, kw, 15)
		if err != nil {
			slog.WarnContext(ctx, "skill search failed for keyword", "keyword", kw, "error", err)
			continue
		}
		for _, r := range results {
			if !seen[r.SkillMDPath] {
				seen[r.SkillMDPath] = true
				allResults = append(allResults, r)
			}
		}
	}

	if len(allResults) > skillCountThreshold {
		allResults = allResults[:skillCountThreshold]
	}
	return allResults
}

func (s *ChatService) resolveLightModel(ctx context.Context) (*models.Model, *models.ModelProvider, error) {
	mdl, err := gorm.G[models.Model](s.db).
		Where("light = true AND deleted_at IS NULL").
		Order("created_at ASC").
		First(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("no light model found: %w", err)
	}

	prov, err := gorm.G[models.ModelProvider](s.db).
		Where("id = ? AND deleted_at IS NULL", mdl.ProviderID).
		First(ctx)
	if err != nil || prov.APIKey == "" {
		return nil, nil, fmt.Errorf("light model provider not configured: %w", err)
	}
	return &mdl, &prov, nil
}

func buildSkillSelectionContext(session *models.ChatSession, userMessage string, historyMessages []providers.Message) string {
	var sb strings.Builder

	if session.Cwd != nil {
		fmt.Fprintf(&sb, "Project directory: %s\n", *session.Cwd)
	}
	if session.AgentsMD != nil {
		content := *session.AgentsMD
		fmt.Fprintf(&sb, "Project instructions:\n%s\n\n", content)
	}

	if len(historyMessages) > 0 {
		sb.WriteString("Recent conversation:\n")
		start := 0
		if len(historyMessages) > 6 {
			start = len(historyMessages) - 6
		}
		for _, msg := range historyMessages[start:] {
			if msg.Role == models.RoleUser || msg.Role == models.RoleAssistant {
				fmt.Fprintf(&sb, "[%s]: %s\n", msg.Role, msg.Content)
			}
		}
		sb.WriteString("\n")
	}

	fmt.Fprintf(&sb, "Current user message: %s\n", userMessage)
	return sb.String()
}

func parseKeywordLines(content string) []string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	var keywords []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "0123456789.-)#<> ")
		line = strings.TrimSpace(line)
		if line != "" {
			keywords = append(keywords, line)
		}
	}
	if len(keywords) > 5 {
		keywords = keywords[:5]
	}
	return keywords
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

func stripThinkingData(messages []providers.Message) []providers.Message {
	result := make([]providers.Message, len(messages))
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

func buildSystemPrompt(agent *models.Agent, session *models.ChatSession, skillsXML, memoriesXML, todosXML string) (string, error) {
	var sb strings.Builder
	data := map[string]any{
		"DateTime":  time.Now().Format(time.RFC3339),
		"AgentName": agent.Name,
		"AgentID":   agent.ID,
		"Soul":      agent.Soul,
	}
	if session != nil && session.Cwd != nil {
		data["Cwd"] = *session.Cwd
	}
	if session != nil && session.AgentsMD != nil {
		data["AgentsMD"] = *session.AgentsMD
	}
	if skillsXML != "" {
		data["SkillsXML"] = skillsXML
	}
	if memoriesXML != "" {
		data["MemoriesXML"] = memoriesXML
	}
	if todosXML != "" {
		data["TodosXML"] = todosXML
	}
	if err := consts.AgentBasePrompt.Execute(&sb, data); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func (s *ChatService) loadHistoryMessages(ctx context.Context, sessionID, modelID uuid.UUID, thinking bool) ([]providers.Message, error) {
	chatMessages, err := gorm.G[*models.ChatMessage](s.db).
		Where("session_id = ? AND deleted_at IS NULL", sessionID).
		Order("created_at ASC").
		Find(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to find chat messages", "error", err, "sessionId", sessionID)
		return nil, err
	}

	messages := lo.Map(chatMessages, func(cm *models.ChatMessage, _ int) providers.Message {
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

		msg := providers.Message{
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
					msg.ReasoningDurationMillis = ps.ReasoningDurationMillis
					if len(ps.AnthropicThinkingBlocks) > 0 {
						msg.ReasoningBlocks = lo.Map(ps.AnthropicThinkingBlocks, func(b models.AnthropicThinkingBlock, _ int) providers.ReasoningBlock {
							if b.Type == "redacted_thinking" {
								return providers.ReasoningBlock{Signature: b.Data, Redacted: true}
							}
							return providers.ReasoningBlock{Summary: b.Thinking, Signature: b.Signature}
						})
					} else if len(ps.GeminiThinkingBlocks) > 0 {
						msg.ReasoningBlocks = lo.Map(ps.GeminiThinkingBlocks, func(b models.GeminiThinkingData, _ int) providers.ReasoningBlock {
							return providers.ReasoningBlock{Summary: b.Summary, Signature: b.ThoughtSignature}
						})
					} else if len(ps.OpenAIReasoningBlocks) > 0 {
						msg.ReasoningBlocks = lo.Map(ps.OpenAIReasoningBlocks, func(b models.OpenAIReasoningBlock, _ int) providers.ReasoningBlock {
							return providers.ReasoningBlock{Summary: b.Summary, Signature: b.EncryptedContent}
						})
					}
				}
			}
		}

		return msg
	})
	return messages, nil
}

func (s *ChatService) saveUserMessage(ctx context.Context, session *models.ChatSession, content string) (uuid.UUID, error) {
	roundID, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, err
	}
	msg := models.ChatMessage{
		ID:        roundID,
		SessionID: session.ID,
		RoundID:   roundID,
		AgentID:   session.AgentID,
		Role:      models.RoleUser,
		Content:   content,
	}
	if err := gorm.G[models.ChatMessage](s.db).Create(ctx, &msg); err != nil {
		slog.ErrorContext(ctx, "failed to save user message", "error", err, "sessionId", session.ID)
		return uuid.Nil, err
	}
	return roundID, nil
}

func buildChatParams(res *chatResources, messages []providers.Message, data *models.ChatDto) *chat.ChatParams {
	var cwd string
	if res.session.Cwd != nil {
		cwd = *res.session.Cwd
	}
	return &chat.ChatParams{
		BaseURL:                   res.chatProvider.BaseURL,
		APIKey:                    res.chatProvider.APIKey,
		Model:                     res.model.Code,
		Messages:                  messages,
		AgentID:                   res.session.AgentID,
		SessionID:                 res.session.ID,
		ModelID:                   res.model.ID,
		Cwd:                       cwd,
		APIType:                   res.chatProvider.Type,
		Thinking:                  data.Thinking && res.model.Thinking,
		ThinkingLevel:             data.ThinkingLevel,
		AnthropicAdaptiveThinking: res.model.AnthropicAdaptiveThinking,
	}
}

func buildChatMessage(m providers.Message, sessionID, roundID, agentID, modelID uuid.UUID, providerType models.APIType, timestamp time.Time, thinkingLevel string) models.ChatMessage {
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
		RoundID:          roundID,
		AgentID:          agentID,
		Role:             models.MessageRole(m.Role),
		Content:          m.Content,
		ToolCalls:        datatypes.JSON(rawCalls),
		ToolResults:      datatypes.JSON(rawCallResult),
		ModelID:          modelID,
		ReasoningContent: m.ReasoningContent,
		CreatedAt:        timestamp,
	}

	if m.Role == models.RoleAssistant {
		var specificData models.ProviderSpecificData
		specificData.ReasoningDurationMillis = m.ReasoningDurationMillis
		switch providerType {
		case models.APITypeAnthropic:
			specificData.AnthropicThinkingBlocks = lo.Map(m.ReasoningBlocks, func(b providers.ReasoningBlock, _ int) models.AnthropicThinkingBlock {
				if b.Redacted {
					return models.AnthropicThinkingBlock{Type: "redacted_thinking", Data: b.Signature}
				}
				return models.AnthropicThinkingBlock{Type: "thinking", Thinking: b.Summary, Signature: b.Signature}
			})
		case models.APITypeGemini:
			specificData.GeminiThinkingBlocks = lo.Map(m.ReasoningBlocks, func(b providers.ReasoningBlock, _ int) models.GeminiThinkingData {
				return models.GeminiThinkingData{ThoughtSignature: b.Signature, ThinkingLevel: thinkingLevel, Summary: b.Summary}
			})
		case models.APITypeOpenAI:
			specificData.OpenAIReasoningBlocks = lo.Map(m.ReasoningBlocks, func(b providers.ReasoningBlock, _ int) models.OpenAIReasoningBlock {
				return models.OpenAIReasoningBlock{Summary: b.Summary, EncryptedContent: b.Signature}
			})
		}
		if specificData.ReasoningDurationMillis > 0 || len(specificData.AnthropicThinkingBlocks) > 0 || len(specificData.GeminiThinkingBlocks) > 0 || len(specificData.OpenAIReasoningBlocks) > 0 {
			if raw, err := json.Marshal(specificData); err == nil {
				chatMsg.ProviderSpecifics = datatypes.JSON(raw)
			}
		}
	}

	return chatMsg
}

func (s *ChatService) saveMessages(ctx context.Context, session *models.ChatSession, modelID, roundID uuid.UUID, messages []models.ChatMessage, totalTokens int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&messages).Error; err != nil {
			return fmt.Errorf("failed to save assistant messages: %w", err)
		}

		hookCtx := &chat.SessionHookContext{
			SessionID:      session.ID,
			AgentID:        session.AgentID,
			ModelID:        modelID,
			RoundID:        roundID,
			TotalTokens:    totalTokens,
			Session:        session,
			Messages:       messages,
			Tx:             tx,
			SessionUpdates: make(map[string]any),
		}
		if err := chat.RunSessionHooks(ctx, chat.SessionHookAfterMessagesSaved, hookCtx); err != nil {
			return err
		}
		if len(hookCtx.SessionUpdates) > 0 {
			if err := tx.Model(&models.ChatSession{}).
				Where("id = ?", session.ID).
				Updates(hookCtx.SessionUpdates).Error; err != nil {
				return fmt.Errorf("failed to update chat session: %w", err)
			}
		}
		if err := chat.RunSessionHooks(ctx, chat.SessionHookAfterRoundCompleted, hookCtx); err != nil {
			return err
		}
		return nil
	})
}
