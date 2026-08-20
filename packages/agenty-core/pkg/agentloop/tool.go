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

func markNativeShellResults(
	calls conversation.Content,
	results []conversation.ToolResultBlock,
) {
	nativeCallIDs := make(map[string]struct{})
	for _, block := range calls {
		call, ok := block.(conversation.ShellCallBlock)
		if ok && call.CallID != "" {
			nativeCallIDs[call.CallID] = struct{}{}
		}
	}

	for resultIndex := range results {
		_, native := nativeCallIDs[results[resultIndex].ToolUseID]
		for blockIndex, block := range results[resultIndex].Content {
			output, ok := block.(conversation.ShellCallOutputBlock)
			if !ok {
				continue
			}
			output.OpenAINative = &native
			results[resultIndex].Content[blockIndex] = output
		}
	}
}
