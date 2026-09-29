package event

import (
	"context"
	"fmt"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/compaction"
	"github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
	"github.com/masteryyh/agenty-core/pkg/infra/permission"
)

type SessionEvent struct {
	Kind                   string                          `json:"kind"`
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

type CompactionStreamEvent struct {
	Kind string `json:"kind"`
	compaction.Event
}

func NewSessionEventMiddleware(broker *StreamBroker) middleware.Middleware {
	return middleware.Middleware{
		Name: "http-session-events",
		OnEvent: func(ctx context.Context, state *middleware.EventContext) error {
			if state == nil || state.Event == nil {
				return nil
			}
			if broker == nil {
				return fmt.Errorf("HTTP event broker is nil")
			}
			event := *state.Event
			topic := "session:" + event.SessionID.String()
			if projected, ok := projectSessionEvent(event); ok {
				if err := broker.Publish(ctx, topic, projected); err != nil {
					return fmt.Errorf("publish session event: %w", err)
				}
			}
			if projected, ok := projectCompactionEvent(event); ok {
				if err := broker.Publish(ctx, topic, projected); err != nil {
					return fmt.Errorf("publish compaction event: %w", err)
				}
			}
			return nil
		},
	}
}

func projectSessionEvent(event agentloop.Event) (SessionEvent, bool) {
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

	projected := SessionEvent{
		Kind:      "session",
		Type:      SessionEventType(event.Type),
		SessionID: event.SessionID,
		RoundID:   event.RoundID,
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
	if change, ok := event.Payload.(conversation.SessionToolDialectChanged); ok {
		projected.ToolDialect = change.ToolDialect
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

func projectCompactionEvent(event agentloop.Event) (CompactionStreamEvent, bool) {
	var eventType compaction.EventType
	switch event.Type {
	case agentloop.EventCompactionStarted:
		eventType = compaction.EventStarted
	case agentloop.EventCompactionDone:
		eventType = compaction.EventCompleted
	case agentloop.EventCompactionFailed:
		eventType = compaction.EventFailed
	default:
		return CompactionStreamEvent{}, false
	}
	message := ""
	if event.Error != nil {
		message = *event.Error
	}
	return CompactionStreamEvent{
		Kind: "compaction",
		Event: compaction.Event{
			Type:                eventType,
			SessionID:           event.SessionID,
			CompactionID:        event.CompactionID,
			Trigger:             event.Trigger,
			ContextTokensBefore: event.ContextTokensBefore,
			ContextTokensAfter:  event.ContextTokensAfter,
			Usage:               event.Usage,
			Error:               message,
		},
	}, true
}
