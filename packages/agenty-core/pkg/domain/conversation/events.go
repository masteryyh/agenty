package conversation

import (
	"fmt"
	"time"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

const (
	EventSessionStarted           = "session_started"
	EventSessionModelSet          = "session_model_set"
	EventSessionThinkingEffortSet = "session_thinking_effort_set"
	EventSessionCwdSet            = "session_cwd_set"
	EventRoundStarted             = "round_started"
	EventMessageAppended          = "message_appended"
	EventRoundEnded               = "round_ended"
	EventSessionTitleSet          = "session_title_set"
)

type SessionStarted struct {
	SessionID      uuid.UUID             `json:"sessionId"`
	Agent          shared.Slug           `json:"agent"`
	Model          shared.ModelRef       `json:"model"`
	ContextWindow  int64                 `json:"contextWindow"`
	ThinkingEffort shared.ThinkingEffort `json:"thinkingEffort,omitempty"`
	Cwd            *string               `json:"cwd,omitempty"`
	At             time.Time             `json:"occurredAt"`
}

func (SessionStarted) EventType() string {
	return EventSessionStarted
}

func (e SessionStarted) OccurredAt() time.Time {
	return e.At
}

type SessionModelSet struct {
	SessionID     uuid.UUID       `json:"sessionId"`
	Model         shared.ModelRef `json:"model"`
	ContextWindow int64           `json:"contextWindow"`
	At            time.Time       `json:"occurredAt"`
}

func (SessionModelSet) EventType() string {
	return EventSessionModelSet
}

func (e SessionModelSet) OccurredAt() time.Time {
	return e.At
}

type SessionThinkingEffortSet struct {
	SessionID      uuid.UUID             `json:"sessionId"`
	ThinkingEffort shared.ThinkingEffort `json:"thinkingEffort"`
	At             time.Time             `json:"occurredAt"`
}

func (SessionThinkingEffortSet) EventType() string {
	return EventSessionThinkingEffortSet
}

func (e SessionThinkingEffortSet) OccurredAt() time.Time {
	return e.At
}

type SessionCwdSet struct {
	SessionID uuid.UUID `json:"sessionId"`
	Cwd       *string   `json:"cwd"`
	At        time.Time `json:"occurredAt"`
}

func (SessionCwdSet) EventType() string {
	return EventSessionCwdSet
}

func (e SessionCwdSet) OccurredAt() time.Time {
	return e.At
}

type RoundStarted struct {
	SessionID      uuid.UUID             `json:"sessionId"`
	RoundID        uuid.UUID             `json:"roundId"`
	Sequence       int                   `json:"sequence"`
	Model          shared.ModelRef       `json:"model"`
	ContextWindow  int64                 `json:"contextWindow"`
	ThinkingEffort shared.ThinkingEffort `json:"thinkingEffort,omitempty"`
	Cwd            *string               `json:"cwd,omitempty"`
	At             time.Time             `json:"occurredAt"`
}

func (RoundStarted) EventType() string {
	return EventRoundStarted
}

func (e RoundStarted) OccurredAt() time.Time {
	return e.At
}

type MessageAppended struct {
	SessionID uuid.UUID `json:"sessionId"`
	Message   Message   `json:"message"`
	At        time.Time `json:"occurredAt"`
}

func (MessageAppended) EventType() string {
	return EventMessageAppended
}

func (e MessageAppended) OccurredAt() time.Time {
	return e.At
}

type RoundEnded struct {
	SessionID uuid.UUID   `json:"sessionId"`
	RoundID   uuid.UUID   `json:"roundId"`
	Status    RoundStatus `json:"status"`
	Usage     TokenUsage  `json:"usage"`
	Error     *string     `json:"error,omitempty"`
	At        time.Time   `json:"occurredAt"`
}

func (RoundEnded) EventType() string {
	return EventRoundEnded
}

func (e RoundEnded) OccurredAt() time.Time {
	return e.At
}

type SessionTitleSet struct {
	SessionID uuid.UUID `json:"sessionId"`
	Title     string    `json:"title"`
	At        time.Time `json:"occurredAt"`
}

func (SessionTitleSet) EventType() string {
	return EventSessionTitleSet
}

func (e SessionTitleSet) OccurredAt() time.Time {
	return e.At
}

func DecodeEvent(env shared.Envelope) (shared.Event, error) {
	switch env.Type {
	case EventSessionStarted:
		return decodePayload[SessionStarted](env.Payload)
	case EventSessionModelSet:
		return decodePayload[SessionModelSet](env.Payload)
	case EventSessionThinkingEffortSet:
		return decodePayload[SessionThinkingEffortSet](env.Payload)
	case EventSessionCwdSet:
		return decodePayload[SessionCwdSet](env.Payload)
	case EventRoundStarted:
		return decodePayload[RoundStarted](env.Payload)
	case EventMessageAppended:
		return decodePayload[MessageAppended](env.Payload)
	case EventRoundEnded:
		return decodePayload[RoundEnded](env.Payload)
	case EventSessionTitleSet:
		return decodePayload[SessionTitleSet](env.Payload)
	default:
		return nil, fmt.Errorf("conversation: unknown event type %q", env.Type)
	}
}

func DecodeEventLine(line []byte) (shared.Event, error) {
	env, err := shared.DecodeEnvelope(line)
	if err != nil {
		return nil, err
	}
	return DecodeEvent(env)
}

func decodePayload[T shared.Event](payload shared.RawJSON) (shared.Event, error) {
	var ev T
	if err := json.Unmarshal(payload, &ev); err != nil {
		return nil, err
	}
	return ev, nil
}
