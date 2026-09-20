package compaction

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
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
    - Do not continue implementing the task.
    - Do not invent completed work or validation results.
    - Be detailed but concise. Remove repetition while preserving essential
      technical details.
    - Return only the summary using the required structure, without a preamble.
  </constraints>
</session-compaction-request>`

// DefaultMaxOutputTokens is the fallback output budget used by session
// execution when a model does not provide one.
const DefaultMaxOutputTokens int64 = catalog.DefaultMaxOutputTokens

// CompactionThreshold returns the request-token threshold at which the output
// budget and a ten-percent safety window leave enough context for a response.
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

// ShouldCompact reports whether a request should be compacted before model
// invocation.
func ShouldCompact(contextTokens, contextWindow, maxOutputTokens int64) bool {
	threshold := CompactionThreshold(contextWindow, maxOutputTokens)
	return threshold == 0 || contextTokens >= threshold
}

// EstimateRequestTokens exposes the conservative estimate used by the
// compaction policy and middleware.
func EstimateRequestTokens(request modelcall.ModelCallRequest) int64 {
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

func estimateMessageTokens(message modelcall.ModelCallMessage) int64 {
	encoded, _ := json.Marshal(message.Content)
	return 4 + estimateTextTokens(string(encoded))
}

func estimateTextTokens(text string) int64 {
	if text == "" {
		return 0
	}
	return int64((len([]rune(text)) + 3) / 4)
}

// Prepared contains the session resources needed for one compaction. The
// session host supplies the request builder so prompt changes and context
// window fitting remain outside this package.
type Prepared struct {
	Session          *conversation.Session
	Model            modelcall.ModelCallConfig
	InvokeModel      modelcall.InvokeFunc
	SystemPrompt     string
	MaxOutputTokens  int64
	RequestForWindow func(contextWindow, maxOutputTokens int64) modelcall.ModelCallRequest
}

// Executor performs compaction mechanics and emits the resulting events.
// Persistence and delivery are owned by OnEvent middleware consumers.
type Executor struct {
	Emit   agentloop.EventEmitter
	Logger Logger
}

// Logger is the small logging port needed for best-effort failure events.
type Logger interface {
	WarnContext(context.Context, string, ...any)
}

// Compact compacts a prepared session using the requested trigger and target
// model window. The returned event contains the persisted compaction details.
func (executor Executor) Compact(
	ctx context.Context,
	prepared Prepared,
	trigger conversation.CompactionTrigger,
	contextWindow int64,
	maxOutputTokens int64,
) (*conversation.SessionCompacted, error) {
	if prepared.Session == nil {
		return nil, fmt.Errorf("compaction session is nil")
	}
	baseMessages := prepared.Session.ContextMessages()
	if len(baseMessages) == 0 {
		return nil, fmt.Errorf("cannot compact an empty session")
	}

	baseRequest := modelcall.ModelCallRequest{
		SystemPrompt:    prepared.SystemPrompt,
		Messages:        modelMessages(baseMessages),
		MaxOutputTokens: prepared.MaxOutputTokens,
		ReasoningEffort: preparedReasoningEffort(prepared.Session),
	}
	contextTokensBefore := EstimateRequestTokens(baseRequest)
	compactionID := uuid.Must(uuid.NewV7())
	if err := executor.emit(ctx, agentloop.Event{
		Type:                agentloop.EventCompactionStarted,
		CompactionID:        compactionID,
		Trigger:             trigger,
		ContextTokensBefore: contextTokensBefore,
	}); err != nil {
		return nil, err
	}

	response, err := executor.invoke(ctx, prepared, baseRequest)
	if err != nil {
		executor.emitFailure(ctx, prepared.Session.ID, compactionID, trigger, err)
		return nil, fmt.Errorf("invoke compaction conversation: %w", err)
	}

	summary, err := textFromContent(response.Content)
	if err != nil {
		executor.emitFailure(ctx, prepared.Session.ID, compactionID, trigger, err)
		return nil, fmt.Errorf("read compaction summary: %w", err)
	}
	event, err := prepared.Session.Compact(conversation.CompactionInput{
		CompactionID:        compactionID,
		Trigger:             trigger,
		Summary:             summary,
		ContextTokensBefore: contextTokensBefore,
		Usage:               response.Usage,
	})
	if err != nil {
		executor.emitFailure(ctx, prepared.Session.ID, compactionID, trigger, err)
		return nil, fmt.Errorf("record compaction: %w", err)
	}
	if prepared.RequestForWindow != nil {
		compactedRequest := prepared.RequestForWindow(contextWindow, maxOutputTokens)
		event.ContextTokensAfter = EstimateRequestTokens(compactedRequest)
	}

	usage := event.Usage
	if err := executor.emit(ctx, agentloop.Event{
		Type:                agentloop.EventCompactionDone,
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

func (executor Executor) invoke(
	ctx context.Context,
	prepared Prepared,
	baseRequest modelcall.ModelCallRequest,
) (*modelcall.ModelCallResponse, error) {
	messages := append([]modelcall.ModelCallMessage(nil), baseRequest.Messages...)
	messages = append(messages, modelcall.ModelCallMessage{
		Role:    conversation.RoleUser,
		Content: conversation.Text(compactionPrompt),
	})

	request := baseRequest
	request.Messages = messages
	invoke := prepared.InvokeModel
	if invoke != nil {
		response, err := invoke(ctx, prepared.Model, request, nil)
		if err != nil {
			return nil, err
		}
		if err := validateCompactionResponse(response); err != nil {
			return nil, err
		}
		return response, nil
	}

	response, err := modelcall.Call(ctx, prepared.Model, request)
	if err != nil {
		return nil, err
	}
	if err := validateCompactionResponse(response); err != nil {
		return nil, err
	}

	return response, nil
}

func preparedReasoningEffort(session *conversation.Session) shared.ReasoningEffort {
	if len(session.Rounds) == 0 {
		return session.CurrentReasoningEffort
	}
	return session.Rounds[len(session.Rounds)-1].ReasoningEffort
}

func (executor Executor) emit(ctx context.Context, event agentloop.Event) error {
	if executor.Emit == nil {
		return nil
	}
	return executor.Emit(ctx, event)
}

func (executor Executor) emitFailure(
	ctx context.Context,
	sessionID uuid.UUID,
	compactionID uuid.UUID,
	trigger conversation.CompactionTrigger,
	err error,
) {
	message := err.Error()
	if emitErr := executor.emit(ctx, agentloop.Event{
		Type:         agentloop.EventCompactionFailed,
		SessionID:    sessionID,
		CompactionID: compactionID,
		Trigger:      trigger,
		Error:        &message,
	}); emitErr != nil && executor.Logger != nil {
		executor.Logger.WarnContext(ctx, "failed to emit compaction failure", "error", emitErr)
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

func validateCompactionResponse(response *modelcall.ModelCallResponse) error {
	if response.StopReason == modelcall.ModelCallStopReasonError {
		return fmt.Errorf("compaction model stopped with an error")
	}
	if response.StopReason == modelcall.ModelCallStopReasonToolUse {
		return fmt.Errorf("compaction model requested tool use")
	}
	for _, block := range response.Content {
		if _, ok := block.(conversation.ToolUseBlock); ok {
			return fmt.Errorf("compaction model returned a tool call")
		}
	}
	return nil
}

// FitCompactedMessages drops retained conversation details before the session
// host projects persisted messages into model-facing messages.
func FitCompactedMessages(
	messages []conversation.Message,
	contextWindow int64,
	maxOutputTokens int64,
) []conversation.Message {
	if contextWindow <= 0 || !hasCompactionSummary(messages) {
		return messages
	}

	limit := CompactionThreshold(contextWindow, maxOutputTokens)
	for estimateMessagesTokens(messages) >= limit {
		removeIndex := retainedMessageIndex(messages, "retained_assistant")
		if removeIndex < 0 {
			removeIndex = retainedMessageIndex(messages, "retained_user")
		}
		if removeIndex < 0 {
			return messages
		}
		messages = append(messages[:removeIndex], messages[removeIndex+1:]...)
	}
	return messages
}

func modelMessages(messages []conversation.Message) []modelcall.ModelCallMessage {
	projected := make([]modelcall.ModelCallMessage, 0, len(messages))
	for _, message := range messages {
		projected = append(projected, modelcall.ModelCallMessage{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	return projected
}

func estimateMessagesTokens(messages []conversation.Message) int64 {
	return EstimateRequestTokens(modelcall.ModelCallRequest{Messages: modelMessages(messages)})
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
