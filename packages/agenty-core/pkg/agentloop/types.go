package agentloop

import (
	"context"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type StopReason string

const (
	StopReasonEndTurn       StopReason = "end_turn"
	StopReasonMaxTokens     StopReason = "max_tokens"
	StopReasonToolUse       StopReason = "tool_use"
	StopReasonContentFilter StopReason = "content_filter"
	StopReasonError         StopReason = "error"
)

type StreamEventType string

const (
	StreamEventTextDelta      StreamEventType = "text_delta"
	StreamEventReasoningDelta StreamEventType = "reasoning_delta"
	StreamEventToolUseStart   StreamEventType = "tool_use_start"
	StreamEventToolInputDelta StreamEventType = "tool_input_delta"
	StreamEventToolUseDone    StreamEventType = "tool_use_done"
	StreamEventCompleted      StreamEventType = "completed"
)

type ToolDefinition struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	InputSchema JSONSchema `json:"inputSchema"`
	Strict      bool       `json:"strict,omitempty"`
}

type Request struct {
	SystemPrompt          string                 `json:"systemPrompt,omitempty"`
	Messages              []conversation.Message `json:"messages"`
	Tools                 []ToolDefinition       `json:"tools,omitempty"`
	MaxOutputTokens       int64                  `json:"maxOutputTokens"`
	ReasoningEffort       shared.ReasoningEffort `json:"reasoningEffort,omitempty"`
	ReasoningBudgetTokens int64                  `json:"reasoningBudgetTokens,omitempty"`
}

type Response struct {
	ID         string                  `json:"id"`
	Model      string                  `json:"model"`
	Content    conversation.Content    `json:"content"`
	Usage      conversation.TokenUsage `json:"usage"`
	StopReason StopReason              `json:"stopReason"`
}

type StreamEvent struct {
	Type      StreamEventType `json:"type"`
	Index     int             `json:"index,omitempty"`
	Delta     string          `json:"delta,omitempty"`
	ToolUseID string          `json:"toolUseId,omitempty"`
	ToolName  string          `json:"toolName,omitempty"`
	ToolInput shared.RawJSON  `json:"toolInput,omitempty"`
	Response  *Response       `json:"response,omitempty"`
}

type StreamHandler func(StreamEvent) error

type SessionEventType string

const (
	SessionEventRoundStarted    SessionEventType = "round_started"
	SessionEventMessageAppended SessionEventType = "message_appended"
	SessionEventModelStream     SessionEventType = "model_stream"
	SessionEventRoundEnded      SessionEventType = "round_ended"
)

type SessionEvent struct {
	Type      SessionEventType         `json:"type"`
	SessionID uuid.UUID                `json:"sessionId"`
	RoundID   uuid.UUID                `json:"roundId"`
	Sequence  uint64                   `json:"sequence"`
	Iteration int                      `json:"iteration,omitempty"`
	Stream    *StreamEvent             `json:"stream,omitempty"`
	Message   *conversation.Message    `json:"message,omitempty"`
	Status    conversation.RoundStatus `json:"status,omitempty"`
	Usage     *conversation.TokenUsage `json:"usage,omitempty"`
	Error     *string                  `json:"error,omitempty"`
}

type SessionEventHandler func(ctx context.Context, event SessionEvent) error

type Caller interface {
	Invoke(ctx context.Context, request Request) (*Response, error)
	Stream(ctx context.Context, request Request, handler StreamHandler) (*Response, error)
}
