// Package compaction contains the session-context compaction integration. The
// policy and transcript mechanics remain independent from the middleware
// manager, so manual compaction and model switching can reuse the same service.
package compaction

import (
	"context"

	"github.com/masteryyh/agenty-core/pkg/infra/middleware"
)

// NewMiddleware installs automatic compaction at the last request-building
// stage. Per-execution operations are supplied through the hook context by the
// session host, keeping the atomic loop independent from compaction.
func NewMiddleware() middleware.Middleware {
	return middleware.Middleware{
		Name: "compaction",
		BeforeModelCall: func(ctx context.Context, state *middleware.ModelCallContext) error {
			if state == nil || state.Request == nil {
				return nil
			}

			operations, ok := OperationsFromContext(state.Context)
			if !ok || operations.Compact == nil || operations.RebuildRequest == nil {
				return nil
			}

			if operations.Attempted != nil && *operations.Attempted {
				return nil
			}

			if !ShouldCompact(
				EstimateRequestTokens(*state.Request),
				operations.ContextWindow,
				operations.MaxOutputTokens,
			) {
				return nil
			}

			usage, err := operations.Compact(ctx)
			if err != nil {
				return err
			}

			state.RequestUsage = state.RequestUsage.Add(usage)
			if operations.Attempted != nil {
				*operations.Attempted = true
			}

			request, err := operations.RebuildRequest(ctx)
			if err != nil {
				return err
			}
			*state.Request = request
			return nil
		},
	}
}
