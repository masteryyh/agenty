package modelcall

import (
	"context"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

// APIType identifies the wire protocol used for a model request.
type APIType string

const (
	APIOpenAI            APIType = "openai"
	APIOpenAICompletions APIType = "openai_completions"
	APIAnthropic         APIType = "anthropic"
	APIGemini            APIType = "gemini"
)

func (t APIType) Valid() bool {
	switch t {
	case APIOpenAI, APIOpenAICompletions, APIAnthropic, APIGemini:
		return true
	default:
		return false
	}
}

// ModelCallConfig contains only the connection and model capabilities needed for one
// model invocation. It deliberately has no session, repository, or provider
// business state.
type ModelCallConfig struct {
	BaseURL           string
	APIType           APIType
	APIKey            string
	ModelCode         string
	Official          bool
	FreeFormTool      bool
	SupportsReasoning bool
	ReasoningEfforts  []shared.ReasoningEffort
}

type ModelCallStopReason string

const (
	ModelCallStopReasonEndTurn       ModelCallStopReason = "end_turn"
	ModelCallStopReasonMaxTokens     ModelCallStopReason = "max_tokens"
	ModelCallStopReasonToolUse       ModelCallStopReason = "tool_use"
	ModelCallStopReasonContentFilter ModelCallStopReason = "content_filter"
	ModelCallStopReasonError         ModelCallStopReason = "error"
)

type ModelCallStreamEventType string

const (
	ModelCallStreamEventTextDelta      ModelCallStreamEventType = "text_delta"
	ModelCallStreamEventReasoningDelta ModelCallStreamEventType = "reasoning_delta"
	ModelCallStreamEventToolUseStart   ModelCallStreamEventType = "tool_use_start"
	ModelCallStreamEventToolInputDelta ModelCallStreamEventType = "tool_input_delta"
	ModelCallStreamEventToolUseDone    ModelCallStreamEventType = "tool_use_done"
	ModelCallStreamEventCompleted      ModelCallStreamEventType = "completed"
)

type ToolType string

const (
	ToolTypeFunction   ToolType = "function"
	ToolTypeShell      ToolType = "shell"
	ToolTypeApplyPatch ToolType = "apply_patch"
)

type ToolDefinition struct {
	Type        ToolType   `json:"type,omitempty"`
	Destructive bool       `json:"destructive"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	InputSchema JSONSchema `json:"inputSchema"`
	Strict      bool       `json:"strict,omitempty"`
}

// ModelCallMessage is the model-facing projection of a conversation message. Session
// identity, round identity, persistence metadata, and timestamps stay with the
// session host.
type ModelCallMessage struct {
	Role    conversation.Role
	Content conversation.Content
}

type ModelCallRequest struct {
	SystemPrompt          string                 `json:"systemPrompt,omitempty"`
	Messages              []ModelCallMessage     `json:"messages"`
	Tools                 []ToolDefinition       `json:"tools,omitempty"`
	MaxOutputTokens       int64                  `json:"maxOutputTokens"`
	ReasoningEffort       shared.ReasoningEffort `json:"reasoningEffort,omitempty"`
	ReasoningBudgetTokens int64                  `json:"reasoningBudgetTokens,omitempty"`
}

type ModelCallResponse struct {
	ID         string                  `json:"id"`
	Model      string                  `json:"model"`
	Content    conversation.Content    `json:"content"`
	Usage      conversation.TokenUsage `json:"usage"`
	StopReason ModelCallStopReason     `json:"stopReason"`
}

type ModelCallStreamEvent struct {
	Type      ModelCallStreamEventType `json:"type"`
	Index     int                      `json:"index,omitempty"`
	Delta     string                   `json:"delta,omitempty"`
	ToolUseID string                   `json:"toolUseId,omitempty"`
	ToolName  string                   `json:"toolName,omitempty"`
	ToolInput shared.RawJSON           `json:"toolInput,omitempty"`
	Response  *ModelCallResponse       `json:"response,omitempty"`
}

type ModelCallStreamHandler func(ModelCallStreamEvent) error

// InvokeFunc permits focused callers to supply a deterministic invocation in
// tests without giving the loop ownership of model-client state.
type InvokeFunc func(context.Context, ModelCallConfig, ModelCallRequest, ModelCallStreamHandler) (*ModelCallResponse, error)
