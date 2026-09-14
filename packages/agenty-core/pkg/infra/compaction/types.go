package compaction

import (
	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

type EventType string

const (
	EventStarted   EventType = "started"
	EventCompleted EventType = "completed"
	EventFailed    EventType = "failed"
)

type Event struct {
	Type                EventType                      `json:"type"`
	SessionID           uuid.UUID                      `json:"sessionId"`
	CompactionID        uuid.UUID                      `json:"compactionId,omitempty"`
	Trigger             conversation.CompactionTrigger `json:"trigger"`
	ContextTokensBefore int64                          `json:"contextTokensBefore,omitempty"`
	ContextTokensAfter  int64                          `json:"contextTokensAfter,omitempty"`
	Usage               *conversation.TokenUsage       `json:"usage,omitempty"`
	Error               string                         `json:"error,omitempty"`
}
