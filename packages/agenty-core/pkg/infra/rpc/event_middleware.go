package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/compaction"
	"github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
	"github.com/masteryyh/agenty-core/pkg/infra/permission"
)

// NotificationSender writes one protocol notification to a connected client.
type NotificationSender func(context.Context, string, any) error

// SessionEvent is the stable session.event notification payload consumed by
// RPC clients. It is projected from an agentloop.Event by this middleware.
type SessionEvent struct {
	Type                   SessionEventType                `json:"type"`
	SessionID              uuid.UUID                       `json:"sessionId"`
	RoundID                uuid.UUID                       `json:"roundId"`
	Sequence               uint64                          `json:"sequence"`
	Iteration              int                             `json:"iteration,omitempty"`
	Stream                 *modelcall.ModelCallStreamEvent `json:"stream,omitempty"`
	Message                *conversation.Message           `json:"message,omitempty"`
	Status                 conversation.RoundStatus        `json:"status,omitempty"`
	Usage                  *conversation.TokenUsage        `json:"usage,omitempty"`
	Error                  *string                         `json:"error,omitempty"`
	Approval               *permission.Request             `json:"approval,omitempty"`
	Resolution             *permission.Resolution          `json:"resolution,omitempty"`
	Review                 *permission.ReviewEvent         `json:"review,omitempty"`
	PermissionMode         conversation.PermissionMode     `json:"permissionMode,omitempty"`
	PreviousPermissionMode conversation.PermissionMode     `json:"previousPermissionMode,omitempty"`
	ToolDialect            conversation.ToolDialect        `json:"toolDialect,omitempty"`
}

type SessionEventType string

const (
	SessionEventRoundStarted          SessionEventType = "round_started"
	SessionEventMessageAppended       SessionEventType = "message_appended"
	SessionEventModelStream           SessionEventType = "model_stream"
	SessionEventRoundEnded            SessionEventType = "round_ended"
	SessionEventPermissionModeChanged SessionEventType = "permission_mode_changed"
	SessionEventToolDialectChanged    SessionEventType = "tool_dialect_changed"
)

type sessionNotificationMiddleware struct {
	send      NotificationSender
	mu        sync.Mutex
	sequences map[uuid.UUID]uint64
}

// NewSessionNotificationMiddleware projects runtime events into the stable
// session.event and session.compaction notifications consumed by the CLI.
func NewSessionNotificationMiddleware(send NotificationSender) middleware.Middleware {
	notifier := &sessionNotificationMiddleware{
		send:      send,
		sequences: make(map[uuid.UUID]uint64),
	}
	return middleware.Middleware{
		Name:    "session-notifications",
		OnEvent: notifier.onEvent,
	}
}

func (notifier *sessionNotificationMiddleware) onEvent(
	ctx context.Context,
	state *middleware.EventContext,
) error {
	if state == nil || state.Event == nil {
		return nil
	}
	if notifier.send == nil {
		return fmt.Errorf("session notification middleware sender is nil")
	}

	event := *state.Event
	if sessionEvent, ok := notifier.sessionEvent(event); ok {
		if err := notifier.send(ctx, "session.event", sessionEvent); err != nil {
			return fmt.Errorf("send session event: %w", err)
		}
		return nil
	}
	if compactionEvent, ok := compactionEvent(event); ok {
		if err := notifier.send(ctx, "session.compaction", compactionEvent); err != nil {
			return fmt.Errorf("send compaction event: %w", err)
		}
	}
	return nil
}

func (notifier *sessionNotificationMiddleware) sessionEvent(
	event agentloop.Event,
) (SessionEvent, bool) {
	switch event.Type {
	case agentloop.EventRoundStarted,
		agentloop.EventMessageAppended,
		agentloop.EventModelStream,
		agentloop.EventRoundEnded,
		permission.EventRequested,
		permission.EventResolved,
		permission.EventReviewStarted,
		permission.EventReviewResolved,
		agentloop.EventPermissionModeChanged,
		agentloop.EventToolDialectChanged:
	default:
		return SessionEvent{}, false
	}

	notifier.mu.Lock()
	notifier.sequences[event.RoundID]++
	sequence := notifier.sequences[event.RoundID]
	if event.Type == agentloop.EventRoundEnded {
		delete(notifier.sequences, event.RoundID)
	}
	notifier.mu.Unlock()

	projected := SessionEvent{
		Type:      SessionEventType(event.Type),
		SessionID: event.SessionID,
		RoundID:   event.RoundID,
		Sequence:  sequence,
		Iteration: event.Iteration,
		Stream:    notificationStream(event.Stream),
		Message:   event.Message,
		Status:    event.Status,
		Usage:     event.Usage,
		Error:     event.Error,
	}
	if approval, ok := event.Payload.(permission.Request); ok {
		projected.Approval = &approval
	}
	if resolution, ok := event.Payload.(permission.Resolution); ok {
		projected.Resolution = &resolution
	}
	if review, ok := event.Payload.(permission.ReviewEvent); ok {
		projected.Review = &review
	}
	if change, ok := event.Payload.(conversation.SessionPermissionModeChanged); ok {
		projected.PermissionMode = change.PermissionMode
		projected.PreviousPermissionMode = change.PreviousMode
	}
	if _, ok := event.Payload.(conversation.SessionCodexModeEnabled); ok {
		projected.ToolDialect = conversation.ToolDialectCodex
	}
	return projected, true
}

func notificationStream(stream *modelcall.ModelCallStreamEvent) *modelcall.ModelCallStreamEvent {
	if stream == nil {
		return nil
	}

	projected := *stream
	if len(projected.ToolInput) > 0 && !json.Valid(projected.ToolInput) {
		projected.ToolInput = nil
	}
	if projected.Response != nil {
		response := *projected.Response
		response.Content = append(conversation.Content(nil), response.Content...)
		for index, block := range response.Content {
			switch value := block.(type) {
			case conversation.ToolUseBlock:
				if !json.Valid(value.Input) {
					value.Input = []byte("{}")
					response.Content[index] = value
				}
			case conversation.ReasoningBlock:
				if len(value.Extra) > 0 && !json.Valid(value.Extra) {
					value.Extra = nil
					response.Content[index] = value
				}
			}
		}
		projected.Response = &response
	}
	return &projected
}

func compactionEvent(event agentloop.Event) (compaction.Event, bool) {
	var eventType compaction.EventType
	switch event.Type {
	case agentloop.EventCompactionStarted:
		eventType = compaction.EventStarted
	case agentloop.EventCompactionDone:
		eventType = compaction.EventCompleted
	case agentloop.EventCompactionFailed:
		eventType = compaction.EventFailed
	default:
		return compaction.Event{}, false
	}

	message := ""
	if event.Error != nil {
		message = *event.Error
	}
	return compaction.Event{
		Type:                eventType,
		SessionID:           event.SessionID,
		CompactionID:        event.CompactionID,
		Trigger:             event.Trigger,
		ContextTokensBefore: event.ContextTokensBefore,
		ContextTokensAfter:  event.ContextTokensAfter,
		Usage:               event.Usage,
		Error:               message,
	}, true
}
