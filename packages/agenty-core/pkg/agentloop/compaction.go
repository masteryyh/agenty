package agentloop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

const compactionPrompt = `<session-compaction-request>
  <purpose>
    Summarize the conversation so another agent can continue the work without
    reading the messages being compacted.
  </purpose>

  <required-output-structure>
    Task goals:
    - All user goals, requirements, constraints, and acceptance criteria.

    Completed:
    - Work already implemented or decisions already made.
    - Preserve important files, APIs, commands, test results, and evidence.

    Incomplete and next steps:
    - Remaining work, blockers, uncertainties, and concrete next actions.

    Important context:
    - Decisions, assumptions, errors, interfaces, tool results, and facts that
      must not be lost.
  </required-output-structure>

  <constraints>
    - Use only information already present in this conversation.
    - Prefer not to call tools. The existing conversation should contain all
      information required to produce the summary.
    - Do not continue implementing the task.
    - Do not invent completed work or validation results.
    - Be detailed but concise. Remove repetition while preserving essential
      technical details.
    - Return only the summary using the required structure, without a preamble.
  </constraints>
</session-compaction-request>`

func CompactionThreshold(contextWindow, maxOutputTokens int64) int64 {
	if contextWindow <= 0 {
		return 0
	}
	if maxOutputTokens < 0 {
		maxOutputTokens = 0
	}
	outputReservedWindow := contextWindow - maxOutputTokens
	safetyWindow := contextWindow - contextWindow/10
	if outputReservedWindow < safetyWindow {
		return max(0, outputReservedWindow)
	}
	return safetyWindow
}

func ShouldCompact(contextTokens, contextWindow, maxOutputTokens int64) bool {
	threshold := CompactionThreshold(contextWindow, maxOutputTokens)
	return threshold == 0 || contextTokens >= threshold
}

func estimateRequestTokens(request Request) int64 {
	tokens := estimateTextTokens(request.SystemPrompt)
	for _, message := range request.Messages {
		tokens += estimateMessageTokens(message)
	}
	for _, tool := range request.Tools {
		encoded, _ := json.Marshal(tool)
		tokens += estimateTextTokens(string(encoded))
	}
	return tokens
}

func estimateMessageTokens(message conversation.Message) int64 {
	encoded, _ := json.Marshal(message.Content)
	return 4 + estimateTextTokens(string(encoded))
}

func estimateTextTokens(text string) int64 {
	if text == "" {
		return 0
	}
	return int64((len([]rune(text)) + 3) / 4)
}

func (engine *Engine) compactPrepared(
	ctx context.Context,
	prepared *preparedExecution,
	trigger conversation.CompactionTrigger,
) (*conversation.SessionCompacted, error) {
	return engine.compactPreparedForWindow(
		ctx,
		prepared,
		trigger,
		modelContextWindow(prepared),
		prepared.maxOutputTokens,
	)
}

func (engine *Engine) compactPreparedForWindow(
	ctx context.Context,
	prepared *preparedExecution,
	trigger conversation.CompactionTrigger,
	contextWindow int64,
	maxOutputTokens int64,
) (*conversation.SessionCompacted, error) {
	baseMessages := sessionMessages(prepared.session)
	if len(baseMessages) == 0 {
		return nil, fmt.Errorf("cannot compact an empty session")
	}

	baseRequest := Request{
		SystemPrompt:    prepared.systemPrompt,
		Messages:        baseMessages,
		Tools:           engine.toolDefinitions(prepared.freeFormTool),
		MaxOutputTokens: maxOutputTokens,
		ReasoningEffort: preparedReasoningEffort(prepared),
	}
	contextTokensBefore := estimateRequestTokens(baseRequest)
	compactionID := uuid.Must(uuid.NewV7())
	if err := engine.emitCompaction(ctx, CompactionEvent{
		Type:                CompactionEventStarted,
		SessionID:           prepared.session.ID,
		CompactionID:        compactionID,
		Trigger:             trigger,
		ContextTokensBefore: contextTokensBefore,
	}); err != nil {
		return nil, err
	}

	response, err := engine.invokeCompaction(ctx, prepared, compactionID, baseRequest)
	if err != nil {
		engine.emitCompactionFailure(ctx, prepared.session.ID, compactionID, trigger, err)
		return nil, fmt.Errorf("invoke compaction conversation: %w", err)
	}

	summary, err := textFromContent(response.Content)
	if err != nil {
		engine.emitCompactionFailure(ctx, prepared.session.ID, compactionID, trigger, err)
		return nil, fmt.Errorf("read compaction summary: %w", err)
	}
	event, err := prepared.session.Compact(conversation.CompactionInput{
		CompactionID:        compactionID,
		Trigger:             trigger,
		Summary:             summary,
		ContextTokensBefore: contextTokensBefore,
		Usage:               response.Usage,
	})
	if err != nil {
		engine.emitCompactionFailure(ctx, prepared.session.ID, compactionID, trigger, err)
		return nil, fmt.Errorf("record compaction: %w", err)
	}
	compactedRequest := engine.sessionRequestForWindow(prepared, contextWindow, maxOutputTokens)
	event.ContextTokensAfter = estimateRequestTokens(compactedRequest)

	if err := engine.saveProgress(ctx, prepared.session); err != nil {
		engine.emitCompactionFailure(ctx, prepared.session.ID, compactionID, trigger, err)
		return nil, fmt.Errorf("save compaction: %w", err)
	}
	usage := event.Usage
	if err := engine.emitCompaction(ctx, CompactionEvent{
		Type:                CompactionEventCompleted,
		SessionID:           prepared.session.ID,
		CompactionID:        event.CompactionID,
		Trigger:             event.Trigger,
		ContextTokensBefore: event.ContextTokensBefore,
		ContextTokensAfter:  event.ContextTokensAfter,
		Usage:               &usage,
	}); err != nil {
		return nil, err
	}
	return &event, nil
}

func (engine *Engine) invokeCompaction(
	ctx context.Context,
	prepared *preparedExecution,
	compactionID uuid.UUID,
	baseRequest Request,
) (*Response, error) {
	messages := append([]conversation.Message(nil), baseRequest.Messages...)
	messages = append(messages, conversation.Message{
		ID:         shared.NewID(),
		RoundID:    compactionID,
		Role:       conversation.RoleUser,
		Visibility: conversation.MessageHidden,
		Content:    conversation.Text(compactionPrompt),
		CreatedAt:  time.Now().UTC(),
	})

	var totalUsage conversation.TokenUsage
	for iteration := 1; iteration <= maxAgentLoopIterations; iteration++ {
		request := baseRequest
		request.Messages = messages
		response, err := prepared.caller.Invoke(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("invoke compaction iteration %d: %w", iteration, err)
		}
		if response == nil {
			return nil, fmt.Errorf("compaction iteration %d returned an empty response", iteration)
		}

		totalUsage = totalUsage.Add(response.Usage)
		calls := toolCalls(response.Content)
		if len(calls) == 0 {
			if response.StopReason == StopReasonError {
				return nil, fmt.Errorf("compaction model stopped with an error")
			}
			response.Usage = totalUsage
			return response, nil
		}

		messages = append(messages, conversation.Message{
			ID:        shared.NewID(),
			RoundID:   compactionID,
			Role:      conversation.RoleAssistant,
			Content:   response.Content,
			Usage:     &response.Usage,
			CreatedAt: time.Now().UTC(),
		})

		results := engine.tools.ExecuteBatch(ctx, CallContext{
			SessionID: prepared.session.ID,
			RoundID:   compactionID,
			Cwd:       sessionCwd(prepared.session),
		}, calls)
		markNativeShellResults(response.Content, results)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		content := make(conversation.Content, 0, len(results))
		for _, result := range results {
			content = append(content, result)
		}
		messages = append(messages, conversation.Message{
			ID:        shared.NewID(),
			RoundID:   compactionID,
			Role:      conversation.RoleUser,
			Content:   content,
			CreatedAt: time.Now().UTC(),
		})
	}

	return nil, fmt.Errorf("compaction conversation exceeded %d iterations", maxAgentLoopIterations)
}

func preparedReasoningEffort(prepared *preparedExecution) shared.ReasoningEffort {
	if len(prepared.session.Rounds) == 0 {
		return prepared.session.CurrentReasoningEffort
	}
	return prepared.session.Rounds[len(prepared.session.Rounds)-1].ReasoningEffort
}

func sessionCwd(session *conversation.Session) string {
	if session.Cwd == nil {
		return ""
	}
	return *session.Cwd
}

func (engine *Engine) emitCompaction(ctx context.Context, event CompactionEvent) error {
	if engine.compactions == nil {
		return nil
	}
	return engine.compactions(ctx, event)
}

func (engine *Engine) emitCompactionFailure(
	ctx context.Context,
	sessionID uuid.UUID,
	compactionID uuid.UUID,
	trigger conversation.CompactionTrigger,
	err error,
) {
	if emitErr := engine.emitCompaction(ctx, CompactionEvent{
		Type:         CompactionEventFailed,
		SessionID:    sessionID,
		CompactionID: compactionID,
		Trigger:      trigger,
		Error:        err.Error(),
	}); emitErr != nil {
		engine.logger.WarnContext(ctx, "failed to emit compaction failure", "error", emitErr)
	}
}

func textFromContent(content conversation.Content) (string, error) {
	parts := make([]string, 0, len(content))
	for _, block := range content {
		textBlock, ok := block.(conversation.TextBlock)
		if ok && textBlock.Text != "" {
			parts = append(parts, textBlock.Text)
		}
	}
	text := strings.TrimSpace(strings.Join(parts, "\n"))
	if text == "" {
		return "", fmt.Errorf("response did not contain text")
	}
	return text, nil
}

func fitCompactedRequest(request Request, contextWindow int64) Request {
	if contextWindow <= 0 || !hasCompactionSummary(request.Messages) {
		return request
	}

	limit := CompactionThreshold(contextWindow, request.MaxOutputTokens)
	for estimateRequestTokens(request) >= limit {
		removeIndex := retainedMessageIndex(request.Messages, "retained_assistant")
		if removeIndex < 0 {
			removeIndex = retainedMessageIndex(request.Messages, "retained_user")
		}
		if removeIndex < 0 {
			return request
		}
		request.Messages = append(request.Messages[:removeIndex], request.Messages[removeIndex+1:]...)
	}
	return request
}

func hasCompactionSummary(messages []conversation.Message) bool {
	for _, message := range messages {
		if compactionKind(message) == "summary" {
			return true
		}
	}
	return false
}

func retainedMessageIndex(messages []conversation.Message, kind string) int {
	for index, message := range messages {
		if compactionKind(message) == kind {
			return index
		}
	}
	return -1
}

func compactionKind(message conversation.Message) string {
	kind, _ := message.Metadata["compactionKind"].(string)
	return kind
}
