package conversation

import (
	"time"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type RoundStatus string

const (
	RoundPending   RoundStatus = "pending"
	RoundRunning   RoundStatus = "running"
	RoundCompleted RoundStatus = "completed"
	RoundFailed    RoundStatus = "failed"
	RoundCancelled RoundStatus = "cancelled"
)

func (s RoundStatus) Terminal() bool {
	switch s {
	case RoundCompleted, RoundFailed, RoundCancelled:
		return true
	default:
		return false
	}
}

type Round struct {
	ID        uuid.UUID       `json:"id"`
	SessionID uuid.UUID       `json:"sessionId"`
	Sequence  int             `json:"sequence"`
	Status    RoundStatus     `json:"status"`
	Model     shared.ModelRef `json:"model"`
	Cwd       *string         `json:"cwd,omitempty"`
	Messages  []Message       `json:"messages"`
	Error     *string         `json:"error,omitempty"`
	StartedAt time.Time       `json:"startedAt"`
	EndedAt   *time.Time      `json:"endedAt,omitempty"`
}
