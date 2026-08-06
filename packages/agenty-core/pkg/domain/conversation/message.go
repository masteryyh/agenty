package conversation

import (
	"time"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type Message struct {
	ID        uuid.UUID        `json:"id"`
	RoundID   uuid.UUID        `json:"roundId"`
	Role      Role             `json:"role"`
	Content   Content          `json:"content"`
	Model     *shared.ModelRef `json:"model,omitempty"`
	Usage     *TokenUsage      `json:"usage,omitempty"`
	Metadata  shared.Metadata  `json:"metadata,omitempty"`
	CreatedAt time.Time        `json:"createdAt"`
}
