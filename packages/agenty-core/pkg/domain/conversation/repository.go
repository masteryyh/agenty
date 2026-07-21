package conversation

import (
	"context"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

// ListQuery filters and paginates a session listing built from the projection.
type ListQuery struct {
	// AgentSlug, when set, restricts results to one agent's sessions.
	AgentSlug *shared.Slug
	// Limit caps the number of rows returned; zero means the implementation's
	// default.
	Limit int
	// Offset skips the first N rows for pagination.
	Offset int
}

// Repository is the persistence port for the conversation aggregate. An
// implementation appends a session's pending events to its JSONL transcript
// (source of truth) and maintains the SQLite `sessions` projection used by List.
type Repository interface {
	// Load reconstructs a session by replaying its transcript.
	Load(ctx context.Context, id uuid.UUID) (*Session, error)
	// Save durably appends the session's pending events and refreshes its
	// projection, then the caller may ClearPending.
	Save(ctx context.Context, session *Session) error
	// List returns session summaries from the projection.
	List(ctx context.Context, query ListQuery) ([]SessionSummary, error)
	// Delete removes a session's transcript and projection row.
	Delete(ctx context.Context, id uuid.UUID) error
}
