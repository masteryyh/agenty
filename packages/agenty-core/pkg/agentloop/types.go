package agentloop

import (
	"context"

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

type Caller interface {
	Invoke(ctx context.Context, request Request) (*Response, error)
	Stream(ctx context.Context, request Request, handler StreamHandler) (*Response, error)
}
