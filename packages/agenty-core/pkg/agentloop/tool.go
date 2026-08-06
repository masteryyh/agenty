package agentloop

import (
	"context"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

type CallContext struct {
	SessionID uuid.UUID
	RoundID   uuid.UUID
	Cwd       string
}

type Tool interface {
	Definition() ToolDefinition
	Execute(
		ctx context.Context,
		callContext CallContext,
		input []byte,
	) (conversation.Content, error)
}

type ToolRuntime interface {
	Definitions() []ToolDefinition
	ExecuteBatch(
		ctx context.Context,
		callContext CallContext,
		calls []conversation.ToolUseBlock,
	) []conversation.ToolResultBlock
}
