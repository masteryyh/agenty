package compaction

import (
	"context"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
)

const OperationsKey = "operations"

// Operations is the per-execution port used by the compaction middleware.
// It is carried through context so the atomic agent loop does not know about
// compaction policy or persistence.
type Operations struct {
	ContextWindow   int64
	MaxOutputTokens int64
	Attempted       *bool
	Compact         func(context.Context) (conversation.TokenUsage, error)
	RebuildRequest  func(context.Context) (modelcall.ModelCallRequest, error)
}

// WithOperations associates compaction operations with an execution context.
func WithOperations(ctx context.Context, operations Operations) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, OperationsKey, operations)
}

// OperationsFromContext returns the compaction operations associated with ctx.
func OperationsFromContext(ctx context.Context) (Operations, bool) {
	if ctx == nil {
		return Operations{}, false
	}
	operations, ok := ctx.Value(OperationsKey).(Operations)
	return operations, ok
}
