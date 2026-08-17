package conversation

import (
	"time"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type MessageVisibility string

const (
	// MessageVisible is the zero value so transcripts written before visibility
	// was introduced remain visible after replay.
	MessageVisible MessageVisibility = ""
	MessageHidden  MessageVisibility = "hidden"
)

type Message struct {
	ID         uuid.UUID         `json:"id"`
	RoundID    uuid.UUID         `json:"roundId"`
	Role       Role              `json:"role"`
	Visibility MessageVisibility `json:"visibility,omitempty"`
	Content    Content           `json:"content"`
	Model      *shared.ModelRef  `json:"model,omitempty"`
	Usage      *TokenUsage       `json:"usage,omitempty"`
	Metadata   shared.Metadata   `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"createdAt"`
}

func (m Message) IsHidden() bool {
	return m.Visibility == MessageHidden
}
