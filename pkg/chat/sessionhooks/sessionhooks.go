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

package sessionhooks

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/chat"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"github.com/masteryyh/agenty/pkg/utils/signal"
	"gorm.io/gorm"
)

var registerOnce sync.Once

func RegisterAll() {
	registerOnce.Do(func() {
		chat.RegisterSessionHook(chat.SessionHookAfterSessionCreated, "savePreviousSessionMemory", chat.SessionHookOptions{IgnoreError: true}, afterSessionCreated)
		chat.RegisterSessionHook(chat.SessionHookAfterMessagesSaved, "updateSessionUsageMetadata", chat.SessionHookOptions{}, updateSessionUsageMetadata)
		chat.RegisterSessionHook(chat.SessionHookAfterRoundCompleted, "recordRoundTokenUsage", chat.SessionHookOptions{}, recordRoundTokenUsage)
	})
}

func afterSessionCreated(_ context.Context, hookCtx *chat.SessionHookContext) error {
	if hookCtx == nil || hookCtx.AgentID == uuid.Nil {
		return nil
	}
	agentID := hookCtx.AgentID
	safe.GoOnce("save-last-session-memory-"+agentID.String(), func() {
		saveLastSessionAsMemory(signal.GetBaseContext(), agentID)
	})
	return nil
}

func updateSessionUsageMetadata(_ context.Context, hookCtx *chat.SessionHookContext) error {
	if hookCtx == nil || hookCtx.Session == nil || hookCtx.SessionUpdates == nil || hookCtx.ModelID == uuid.Nil {
		return nil
	}

	sessionTokens := hookCtx.ContextTokens
	if sessionTokens <= 0 {
		sessionTokens = hookCtx.TotalTokens
	}

	hookCtx.Session.TokenConsumed = sessionTokens
	hookCtx.Session.ContextTokens = sessionTokens
	hookCtx.Session.LastUsedModel = hookCtx.ModelID
	hookCtx.Session.LastUsedThinkingLevel = &hookCtx.ThinkingLevel
	hookCtx.SessionUpdates["token_consumed"] = sessionTokens
	if sessionTokens > 0 {
		hookCtx.SessionUpdates["context_tokens"] = sessionTokens
	}
	hookCtx.SessionUpdates["last_used_model"] = hookCtx.ModelID
	hookCtx.SessionUpdates["last_used_thinking_level"] = hookCtx.ThinkingLevel
	return nil
}

func recordRoundTokenUsage(ctx context.Context, hookCtx *chat.SessionHookContext) error {
	if hookCtx == nil || hookCtx.SessionID == uuid.Nil || hookCtx.AgentID == uuid.Nil || hookCtx.ModelID == uuid.Nil || hookCtx.RoundID == uuid.Nil {
		return nil
	}

	db := hookCtx.Tx
	if db == nil {
		db = conn.GetDB()
	}

	usage := models.ChatRoundTokenUsage{
		SessionID:   hookCtx.SessionID,
		AgentID:     hookCtx.AgentID,
		ModelID:     hookCtx.ModelID,
		RoundID:     hookCtx.RoundID,
		TotalTokens: hookCtx.TotalTokens,
	}
	return gorm.G[models.ChatRoundTokenUsage](db).Create(ctx, &usage)
}

func saveLastSessionAsMemory(ctx context.Context, agentID uuid.UUID) {
	db := conn.GetDB()

	prevSession, err := gorm.G[models.ChatSession](db).
		Where("agent_id = ? AND deleted_at IS NULL", agentID).
		Order("created_at DESC").
		Offset(1).
		First(ctx)
	if err != nil {
		return
	}

	_, err = gorm.G[models.KnowledgeItem](db).
		Where("source_session_id = ? AND deleted_at IS NULL", prevSession.ID).
		First(ctx)
	if err == nil {
		return
	}

	messages, err := gorm.G[*models.ChatMessage](db).
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
		fmt.Fprintf(&sb, "[%s]: %s\n\n", msg.Role, content)
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

	if _, err := services.GetKnowledgeService().CreateSessionMemory(ctx, agentID, prevSession.ID, title, sessionText); err != nil {
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
