package skill

import (
	"context"
	"strings"

	infraMiddleware "github.com/masteryyh/agenty-core/pkg/infra/middleware"
)

// NewMiddleware exposes the already-scanned skill registry to a session. File
// discovery remains owned by Registry; this adapter only joins the registry
// with the execution lifecycle.
func NewMiddleware(registry *Registry) infraMiddleware.Middleware {
	middleware := infraMiddleware.Middleware{Name: "skill"}
	if registry == nil {
		return middleware
	}

	middleware.BeforeSessionStart = func(_ context.Context, state *infraMiddleware.SessionStartContext) error {
		if state == nil || state.SystemPrompt == nil {
			return nil
		}
		section := strings.TrimSpace(registry.PromptSection())
		if section == "" {
			return nil
		}
		base := strings.TrimRight(*state.SystemPrompt, " \t\r\n")
		if base == "" {
			*state.SystemPrompt = section
			return nil
		}
		*state.SystemPrompt = base + "\n\n" + section
		return nil
	}

	return middleware
}
