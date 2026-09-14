package storage

import (
	"context"
	"fmt"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/middleware"
)

// SessionSaver is the write port used by session persistence middleware.
type SessionSaver interface {
	Save(context.Context, *conversation.Session) error
}

// NewSessionMiddleware persists pending session events before later event
// consumers, such as CLI notification delivery, observe them.
func NewSessionMiddleware(saver SessionSaver) middleware.Middleware {
	return middleware.Middleware{
		Name: "session-storage",
		OnEvent: func(ctx context.Context, state *middleware.EventContext) error {
			if state == nil || state.Session == nil || len(state.Session.PendingEvents()) == 0 {
				return nil
			}

			if saver == nil {
				return fmt.Errorf("session storage middleware saver is nil")
			}

			if err := saver.Save(ctx, state.Session); err != nil {
				return fmt.Errorf("save session events: %w", err)
			}
			state.Session.ClearPending()
			return nil
		},
	}
}
