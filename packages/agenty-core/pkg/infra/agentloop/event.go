package agentloop

import (
	"context"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
)

// EventType identifies a provider-neutral runtime event. Infrastructure
// packages may add event types while sharing this envelope and emitter port.
type EventType string

const (
	EventRoundStarted          EventType = "round_started"
	EventMessageAppended       EventType = "message_appended"
	EventModelStream           EventType = "model_stream"
	EventRoundEnded            EventType = "round_ended"
	EventAssistantResponse     EventType = "assistant_response"
	EventToolResults           EventType = "tool_results"
	EventSessionChanged        EventType = "session_changed"
	EventPermissionModeChanged EventType = "permission_mode_changed"
	EventToolDialectChanged    EventType = "tool_dialect_changed"
	EventCompactionStarted     EventType = "compaction_started"
	EventCompactionDone        EventType = "compaction_completed"
	EventCompactionFailed      EventType = "compaction_failed"
)

// Event carries the data produced during agent execution. Session lifecycle
// fields are populated by the session host before infrastructure consumers
// receive the event.
type Event struct {
	Type      EventType
	SessionID uuid.UUID
	RoundID   uuid.UUID
	Iteration int

	Stream      *modelcall.ModelCallStreamEvent
	Response    *modelcall.ModelCallResponse
	ToolResults conversation.Content
	Message     *conversation.Message
	Status      conversation.RoundStatus
	Usage       *conversation.TokenUsage
	Error       *string
	// Payload carries an infrastructure-owned event type's data. Its producer
	// and consumers share a concrete type without coupling the loop to it.
	Payload any

	CompactionID        uuid.UUID
	Trigger             conversation.CompactionTrigger
	ContextTokensBefore int64
	ContextTokensAfter  int64
}

// EventEmitter publishes one runtime event. It is a synchronous port so a
// caller can stop when an event consumer rejects the operation.
type EventEmitter func(context.Context, Event) error
