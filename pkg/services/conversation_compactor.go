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
	"os"
	"path/filepath"
	"strings"
	"sync"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/chat"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/providers"
	"github.com/masteryyh/agenty/pkg/tools"
	tiktoken "github.com/pkoukk/tiktoken-go"
	tiktokenloader "github.com/pkoukk/tiktoken-go-loader"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	compactionSummaryBudget      int64 = 10000
	compactionMinWorkingHeadroom int64 = 24000
	microcompactMinContentTokens       = 3000
	microcompactKeptTokens             = 400
)

var microcompactToolNames = map[string]bool{
	"read_file":         true,
	"write_file":        true,
	"replace_in_file":   true,
	"list_directory":    true,
	"run_shell_command": true,
	"search":            true,
	"fetch":             true,
}

type ConversationCompactor struct {
	db *gorm.DB
}

type messageGroup struct {
	Messages []providers.Message
	Tokens   int64
}

type compactionKind string

const (
	compactionKindMicro       compactionKind = "microcompact"
	compactionKindAgent       compactionKind = "agent_compact"
	compactionKindActiveRound compactionKind = "active_round_agent_compact"
)

type compactionTranscript struct {
	Version         int                 `json:"version"`
	SessionID       uuid.UUID           `json:"sessionId"`
	AgentID         uuid.UUID           `json:"agentId"`
	ModelID         uuid.UUID           `json:"modelId"`
	SourceTokens    int64               `json:"sourceTokens"`
	ThresholdTokens int64               `json:"thresholdTokens"`
	OlderPrefix     []providers.Message `json:"olderPrefix"`
	RecentTail      []providers.Message `json:"recentTail"`
}

var (
	conversationCompactor     *ConversationCompactor
	conversationCompactorOnce sync.Once
	tokenEncoding             *tiktoken.Tiktoken
	tokenEncodingErr          error
	tokenEncodingOnce         sync.Once
)

func GetConversationCompactor() *ConversationCompactor {
	conversationCompactorOnce.Do(func() {
		conversationCompactor = &ConversationCompactor{
			db: conn.GetDB(),
		}
	})
	return conversationCompactor
}

func (c *ConversationCompactor) CompactBeforeModelCall(ctx context.Context, params *chat.ChatParams, req *providers.ChatRequest, iteration int, emit func(providers.StreamEvent)) error {
	if c == nil || params == nil || req == nil || params.ModelID == uuid.Nil {
		return nil
	}

	model, err := gorm.G[models.Model](c.db).
		Where("id = ? AND deleted_at IS NULL", params.ModelID).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if model.ContextWindow <= 0 {
		return nil
	}

	threshold := compactThreshold(int64(model.ContextWindow), params.Thinking || model.Thinking)
	sourceTokens := estimateRequestTokensFromCurrentContext(params, req)
	if sourceTokens <= threshold {
		return nil
	}

	emitCompactionEvent(emit, providers.EventCompactionStart)
	compactedMessages, microChanged := MicroCompact(req.Messages)
	req.Messages = compactedMessages
	compactedTokens := estimateRequestTokensFromCurrentContext(params, req)

	if compactedTokens <= threshold {
		req.Messages = compactedMessages
		if microChanged {
			if err := c.saveCompaction(ctx, params, compactionKindMicro, req.Messages, "", sourceTokens, compactedTokens, threshold); err != nil {
				return err
			}
		}
		emitCompactionEvent(emit, providers.EventCompactionDone)
		return nil
	}

	prefix, tail, compactedTail := selectCompactionGroups(req.Messages, compactedMessages, tailBudget(threshold))
	if len(prefix) == 0 {
		return fmt.Errorf("conversation context exceeds model budget and no compactable prefix was found")
	}

	summary, err := c.AgentCompact(ctx, params, prefix, tail, &model, threshold, sourceTokens)
	if err != nil {
		return err
	}

	req.Messages = assembleCompactedMessages(compactedMessages, compactedTail, summary)
	finalTokens := estimateRequestTokens(req)
	kind := compactionKindAgent
	if iteration > 1 {
		kind = compactionKindActiveRound
	}
	if err := c.saveCompaction(ctx, params, kind, req.Messages, summary, sourceTokens, finalTokens, threshold); err != nil {
		return err
	}
	emitCompactionEvent(emit, providers.EventCompactionDone)
	return nil
}

func (c *ConversationCompactor) AgentCompactIfNeeded(ctx context.Context, params *chat.ChatParams, req *providers.ChatRequest, model *models.Model, force bool) (bool, error) {
	if c == nil || params == nil || req == nil || model == nil || model.ContextWindow <= 0 {
		return false, nil
	}

	threshold := compactThreshold(int64(model.ContextWindow), params.Thinking || model.Thinking)
	sourceTokens := estimateRequestTokens(req)
	if sourceTokens <= threshold && !force {
		return false, nil
	}

	compactedMessages, _ := MicroCompact(req.Messages)
	if force {
		prefix := groupMessages(nonSystemMessages(req.Messages))
		if len(prefix) == 0 {
			return false, nil
		}
		summary, err := c.AgentCompact(ctx, params, prefix, nil, model, threshold, sourceTokens)
		if err != nil {
			return false, err
		}
		req.Messages = assembleCompactedMessages(compactedMessages, nil, summary)
		finalTokens := estimateRequestTokens(req)
		if err := c.saveCompaction(ctx, params, compactionKindAgent, req.Messages, summary, sourceTokens, finalTokens, threshold); err != nil {
			return false, err
		}
		return true, nil
	}

	budget := tailBudget(threshold)
	prefix, tail, compactedTail := selectCompactionGroups(req.Messages, compactedMessages, budget)
	if len(prefix) == 0 {
		return false, nil
	}

	summary, err := c.AgentCompact(ctx, params, prefix, tail, model, threshold, sourceTokens)
	if err != nil {
		return false, err
	}

	req.Messages = assembleCompactedMessages(compactedMessages, compactedTail, summary)
	finalTokens := estimateRequestTokens(req)
	if err := c.saveCompaction(ctx, params, compactionKindAgent, req.Messages, summary, sourceTokens, finalTokens, threshold); err != nil {
		return false, err
	}
	return true, nil
}

func emitCompactionEvent(emit func(providers.StreamEvent), eventType providers.StreamEventType) {
	if emit == nil {
		return
	}
	emit(providers.StreamEvent{Type: eventType})
}

func compactThreshold(contextWindow int64, thinking bool) int64 {
	var reserve int64
	if thinking {
		reserve = max(int64(32000), min(contextWindow*8/100, int64(128000)))
	} else {
		reserve = max(int64(16000), min(contextWindow*5/100, int64(64000)))
	}
	safetyBuffer := min(max(int64(3000), contextWindow/100), int64(12000))
	available := contextWindow - reserve - safetyBuffer
	threshold := max(max(available*95/100, contextWindow/2), 1000)
	return threshold
}

func tailBudget(effectiveBudget int64) int64 {
	budget := min(effectiveBudget*45/100, effectiveBudget-compactionSummaryBudget-compactionMinWorkingHeadroom)
	if budget < 8000 {
		budget = effectiveBudget / 2
	}
	if budget < 1000 {
		budget = 1000
	}
	return budget
}

func MicroCompact(messages []providers.Message) ([]providers.Message, bool) {
	result := make([]providers.Message, len(messages))
	copy(result, messages)

	changed := false
	for i := range result {
		if result[i].Role != models.RoleTool || result[i].ToolResult == nil {
			continue
		}
		name := result[i].ToolResult.Name
		if !microcompactToolNames[name] || estimateTextTokens(result[i].ToolResult.Content) < microcompactMinContentTokens {
			continue
		}
		content := compactToolResultContent(*result[i].ToolResult)
		result[i].Content = content
		result[i].ToolResult.Content = content
		changed = true
	}
	return result, changed
}

func nonSystemMessages(messages []providers.Message) []providers.Message {
	result := make([]providers.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == models.RoleSystem {
			continue
		}
		result = append(result, msg)
	}
	return result
}

func compactToolResultContent(result models.ToolResult) string {
	content := result.Content
	originalTokens := estimateTextTokens(content)
	head, tail := splitKeptTokenContent(content, microcompactKeptTokens)

	var sb strings.Builder
	fmt.Fprintf(&sb, "[Tool result compacted: %s", result.Name)
	descriptor := toolResultDescriptor(content)
	if descriptor != "" {
		fmt.Fprintf(&sb, ", %s", descriptor)
	}
	fmt.Fprintf(&sb, ", original %d tokens", originalTokens)
	if result.IsError {
		sb.WriteString(", error=true")
	}
	sb.WriteString("]\n")
	if head != "" {
		sb.WriteString("Kept head:\n")
		sb.WriteString(head)
		sb.WriteString("\n")
	}
	if tail != "" && tail != head {
		sb.WriteString("Kept tail:\n")
		sb.WriteString(tail)
		sb.WriteString("\n")
	}
	return sb.String()
}

func splitKeptTokenContent(content string, keepTokens int) (string, string) {
	encoding, err := o200kEncoding()
	if err != nil {
		return splitKeptByteContent(content, keepTokens*4)
	}
	tokens := encoding.Encode(content, nil, nil)
	if len(tokens) <= keepTokens*2 {
		return content, ""
	}
	return encoding.Decode(tokens[:keepTokens]), encoding.Decode(tokens[len(tokens)-keepTokens:])
}

func splitKeptByteContent(content string, keepBytes int) (string, string) {
	if len(content) <= keepBytes*2 {
		return content, ""
	}
	return content[:keepBytes], content[len(content)-keepBytes:]
}

func toolResultDescriptor(content string) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return ""
	}
	keys := []string{"path", "url", "query", "command", "exitCode", "statusCode", "resultCount", "bytes", "truncated"}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}
	return strings.Join(parts, ", ")
}

func selectRecentTail(messages []providers.Message, tokenBudget int64) ([]messageGroup, []messageGroup) {
	groups := groupMessages(messages)
	var tail []messageGroup
	var total int64
	cut := len(groups)
	for i := len(groups) - 1; i >= 0; i-- {
		group := groups[i]
		if len(tail) > 0 && total+group.Tokens > tokenBudget {
			break
		}
		total += group.Tokens
		tail = append([]messageGroup{group}, tail...)
		cut = i
	}
	return groups[:cut], tail
}

func selectCompactionGroups(originalMessages, compactedMessages []providers.Message, tokenBudget int64) ([]messageGroup, []messageGroup, []messageGroup) {
	compactedPrefix, compactedTail := selectRecentTail(nonSystemMessages(compactedMessages), tokenBudget)
	originalGroups := groupMessages(nonSystemMessages(originalMessages))
	if len(originalGroups) != len(compactedPrefix)+len(compactedTail) {
		originalPrefix, originalTail := selectRecentTail(nonSystemMessages(originalMessages), tokenBudget)
		return originalPrefix, originalTail, compactedTail
	}
	cut := len(compactedPrefix)
	return originalGroups[:cut], originalGroups[cut:], compactedTail
}

func groupMessages(messages []providers.Message) []messageGroup {
	groups := make([]messageGroup, 0, len(messages))
	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		group := messageGroup{Messages: []providers.Message{msg}}
		if msg.Role == models.RoleAssistant && len(msg.ToolCalls) > 0 {
			callIDs := make(map[string]bool, len(msg.ToolCalls))
			for _, call := range msg.ToolCalls {
				callIDs[call.ID] = true
			}
			for i+1 < len(messages) && messages[i+1].Role == models.RoleTool {
				next := messages[i+1]
				if next.ToolResult != nil && !callIDs[next.ToolResult.CallID] {
					break
				}
				group.Messages = append(group.Messages, next)
				i++
			}
		}
		group.Tokens = estimateMessagesTokens(group.Messages)
		groups = append(groups, group)
	}
	return groups
}

func (c *ConversationCompactor) AgentCompact(ctx context.Context, params *chat.ChatParams, prefix, tail []messageGroup, currentModel *models.Model, thresholdTokens, sourceTokens int64) (string, error) {
	tempDir, err := os.MkdirTemp("", "agenty-compact-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	transcript := compactionTranscript{
		Version:         1,
		SessionID:       params.SessionID,
		AgentID:         params.AgentID,
		ModelID:         params.ModelID,
		SourceTokens:    sourceTokens,
		ThresholdTokens: thresholdTokens,
		OlderPrefix:     flattenMessageGroups(prefix),
		RecentTail:      flattenMessageGroups(tail),
	}
	raw, err := json.MarshalIndent(transcript, "", "  ")
	if err != nil {
		return "", err
	}
	transcriptPath := filepath.Join(tempDir, "conversation.json")
	if err := os.WriteFile(transcriptPath, raw, 0600); err != nil {
		return "", err
	}

	thinking := params.Thinking
	if currentModel != nil {
		thinking = thinking || currentModel.Thinking
	}

	sessionID, err := c.createCompactionSession(ctx, params.ModelID, tempDir)
	if err != nil {
		return "", err
	}
	defer c.cleanupCompactionSession(context.WithoutCancel(ctx), sessionID)

	// TODO: teach the normal chat flow to use a compaction-safe tool registry once tool capability metadata, command detection, and sandbox controls exist.
	result, err := GetChatService().Chat(ctx, sessionID, &models.ChatDto{
		ModelID:       params.ModelID,
		Message:       buildAgentCompactUserPrompt(prefix, tail),
		Thinking:      thinking,
		ThinkingLevel: params.ThinkingLevel,
	})
	if err != nil {
		return "", fmt.Errorf("failed to compact conversation with agent: %w", err)
	}
	for i := len(result) - 1; i >= 0; i-- {
		if result[i].Role != models.RoleAssistant {
			continue
		}
		summary := strings.TrimSpace(result[i].Content)
		if summary == "" {
			continue
		}
		return summary, nil
	}
	return "", fmt.Errorf("failed to compact conversation with agent: empty summary")
}

func (c *ConversationCompactor) createCompactionSession(ctx context.Context, modelID uuid.UUID, cwd string) (uuid.UUID, error) {
	agent, err := gorm.G[models.Agent](c.db).
		Where("is_default = ? AND deleted_at IS NULL", true).
		First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return uuid.Nil, customerrors.ErrAgentNotFound
		}
		return uuid.Nil, err
	}

	session := &models.ChatSession{
		AgentID:       agent.ID,
		LastUsedModel: modelID,
		Cwd:           &cwd,
	}
	if err := gorm.G[models.ChatSession](c.db).Create(ctx, session); err != nil {
		return uuid.Nil, err
	}
	return session.ID, nil
}

func (c *ConversationCompactor) cleanupCompactionSession(ctx context.Context, sessionID uuid.UUID) {
	if sessionID == uuid.Nil {
		return
	}
	if err := GetSkillService().DropSessionTable(ctx, sessionID); err != nil {
		slog.WarnContext(ctx, "failed to drop compaction session skills table", "sessionId", sessionID, "error", err)
	}
	if err := c.db.WithContext(ctx).
		Model(&models.ChatSession{}).
		Where("id = ?", sessionID).
		Update("deleted_at", conn.NowExpr()).Error; err != nil {
		slog.WarnContext(ctx, "failed to delete compaction session", "sessionId", sessionID, "error", err)
	}
}

func buildAgentCompactUserPrompt(_ []messageGroup, _ []messageGroup) string {
	var sb strings.Builder
	fmt.Fprintln(&sb, consts.CompactPrompt)
	fmt.Fprintln(&sb)
	fmt.Fprint(&sb, "The working directory contains conversation.json. Study it with tools and produce a compact summary of olderPrefix for future model calls.\n")
	fmt.Fprint(&sb, "Use recentTail only to avoid repeating context that will remain raw after the summary.\n")
	fmt.Fprint(&sb, "Write the summary in the user's preferred language shown by the conversation context.\n")
	fmt.Fprint(&sb, "Use exactly three section headings translated into that same language:\n")
	fmt.Fprint(&sb, "- Completed task goals\n")
	fmt.Fprint(&sb, "- Key methods used and key decisions made\n")
	fmt.Fprint(&sb, "- Unfinished task goals\n")
	fmt.Fprint(&sb, "Under each heading, use markdown unordered list items only. If a section has no content, emit a single bullet saying none in that same language.\n")
	fmt.Fprint(&sb, "Preserve concrete paths, commands, identifiers, error messages, tests, important tool calls and command results, durable user or project constraints, failures and fixes, and unresolved risks when they matter. Do not mention the temporary file path.")
	return sb.String()
}

func flattenMessageGroups(groups []messageGroup) []providers.Message {
	var result []providers.Message
	for _, group := range groups {
		result = append(result, group.Messages...)
	}
	return result
}

func assembleCompactedMessages(messages []providers.Message, tail []messageGroup, summary string) []providers.Message {
	result := make([]providers.Message, 0, len(messages)+1)
	for _, msg := range messages {
		if msg.Role != models.RoleSystem {
			continue
		}
		result = append(result, msg)
	}
	result = append(result, providers.Message{
		Role:    models.RoleAssistant,
		Content: "<conversation-summary>\n" + summary + "\n</conversation-summary>",
	})
	for _, group := range tail {
		result = append(result, group.Messages...)
	}
	return result
}

func (c *ConversationCompactor) saveCompaction(ctx context.Context, params *chat.ChatParams, kind compactionKind, messages []providers.Message, summary string, sourceTokens, compactedTokens, thresholdTokens int64) error {
	rawMessages, err := json.Marshal(messages)
	if err != nil {
		return err
	}

	roundID, messageID := c.latestPersistedBoundary(ctx, params.SessionID)
	compaction := models.ChatCompaction{
		SessionID:               params.SessionID,
		AgentID:                 params.AgentID,
		ModelID:                 params.ModelID,
		Type:                    string(kind),
		CompactedUntilRoundID:   roundID,
		CompactedUntilMessageID: messageID,
		Summary:                 summary,
		CompactedMessages:       datatypes.JSON(rawMessages),
		SourceTokenEstimate:     sourceTokens,
		CompactedTokenEstimate:  compactedTokens,
		ThresholdTokens:         thresholdTokens,
	}

	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&compaction).Error; err != nil {
			return err
		}
		return tx.Model(&models.ChatSession{}).
			Where("id = ?", params.SessionID).
			Update("active_compaction_id", compaction.ID).Error
	})
}

func (c *ConversationCompactor) latestPersistedBoundary(ctx context.Context, sessionID uuid.UUID) (*uuid.UUID, *uuid.UUID) {
	msg, err := gorm.G[models.ChatMessage](c.db).
		Where("session_id = ? AND deleted_at IS NULL", sessionID).
		Order("created_at DESC").
		First(ctx)
	if err != nil {
		return nil, nil
	}
	return &msg.RoundID, &msg.ID
}

func estimateRequestTokens(req *providers.ChatRequest) int64 {
	return estimateMessagesTokens(req.Messages) + estimateToolDefinitionTokens(req.APIType, req.Tools)
}

func estimateRequestTokensFromCurrentContext(params *chat.ChatParams, req *providers.ChatRequest) int64 {
	if params == nil || req == nil || params.ContextTokens <= 0 || params.NewMessageStart < 0 || params.NewMessageStart > len(req.Messages) {
		return estimateRequestTokens(req)
	}
	return params.ContextTokens + estimateMessagesTokens(req.Messages[params.NewMessageStart:])
}

func estimateMessagesTokens(messages []providers.Message) int64 {
	var total int64
	for _, msg := range messages {
		raw, err := json.Marshal(msg)
		if err != nil {
			total += estimateTextTokens(msg.Content)
			continue
		}
		total += estimateTextTokens(string(raw))
	}
	return total
}

func estimateToolDefinitionTokens(apiType models.APIType, defs []tools.ToolDefinition) int64 {
	if len(defs) == 0 {
		return 0
	}
	raw, err := json.Marshal(providers.ToolSchemaForTokenEstimate(apiType, defs))
	if err != nil {
		return 0
	}
	return estimateTextTokens(string(raw))
}

func estimateTextTokens(text string) int64 {
	if text == "" {
		return 0
	}
	encoding, err := o200kEncoding()
	if err != nil {
		slog.Warn("failed to initialize o200k_base token estimator", "error", err)
		return int64(max(1, (len(text)+3)/4))
	}
	return int64(len(encoding.Encode(text, nil, nil)))
}

func o200kEncoding() (*tiktoken.Tiktoken, error) {
	tokenEncodingOnce.Do(func() {
		tiktoken.SetBpeLoader(tiktokenloader.NewOfflineLoader())
		tokenEncoding, tokenEncodingErr = tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
	})
	return tokenEncoding, tokenEncodingErr
}
