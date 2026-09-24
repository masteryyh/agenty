package rpc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
	"github.com/masteryyh/agenty-core/pkg/infra/permission"
)

func TestSessionNotificationMiddlewareProjectsEventsWithRoundSequences(t *testing.T) {
	sessionID := uuid.Must(uuid.NewV7())
	roundID := uuid.Must(uuid.NewV7())
	var sent []struct {
		method  string
		payload any
	}
	notifier := NewSessionNotificationMiddleware(func(_ context.Context, method string, payload any) error {
		sent = append(sent, struct {
			method  string
			payload any
		}{method: method, payload: payload})
		return nil
	})

	for _, event := range []agentloop.Event{
		{Type: agentloop.EventRoundStarted, SessionID: sessionID, RoundID: roundID},
		{Type: agentloop.EventModelStream, SessionID: sessionID, RoundID: roundID, Iteration: 1,
			Stream: &modelcall.ModelCallStreamEvent{Type: modelcall.ModelCallStreamEventTextDelta, Delta: "hello"}},
		{Type: agentloop.EventRoundEnded, SessionID: sessionID, RoundID: roundID,
			Status: conversation.RoundCompleted},
	} {
		event := event
		if err := notifier.OnEvent(t.Context(), &middleware.EventContext{Event: &event}); err != nil {
			t.Fatal(err)
		}
	}

	if len(sent) != 3 {
		t.Fatalf("notifications = %d, want 3", len(sent))
	}
	for index, notification := range sent {
		if notification.method != "session.event" {
			t.Fatalf("notification %d method = %q", index, notification.method)
		}
		event, ok := notification.payload.(SessionEvent)
		if !ok {
			t.Fatalf("notification %d payload = %T", index, notification.payload)
		}
		if event.Sequence != uint64(index+1) || event.SessionID != sessionID || event.RoundID != roundID {
			t.Fatalf("notification %d event = %+v", index, event)
		}
	}

	nextRound := uuid.Must(uuid.NewV7())
	event := agentloop.Event{Type: agentloop.EventRoundStarted, SessionID: sessionID, RoundID: nextRound}
	if err := notifier.OnEvent(t.Context(), &middleware.EventContext{Event: &event}); err != nil {
		t.Fatal(err)
	}
	next, ok := sent[3].payload.(SessionEvent)
	if !ok || next.Sequence != 1 {
		t.Fatalf("next round event = %+v", sent[3].payload)
	}
}

func TestSessionNotificationMiddlewareSanitizesUntrustedStreamJSON(t *testing.T) {
	sessionID := uuid.Must(uuid.NewV7())
	roundID := uuid.Must(uuid.NewV7())
	var sent []SessionEvent
	notifier := NewSessionNotificationMiddleware(func(_ context.Context, _ string, payload any) error {
		if _, err := json.Marshal(payload); err != nil {
			return err
		}
		sent = append(sent, payload.(SessionEvent))
		return nil
	})

	invalidResponse := &modelcall.ModelCallResponse{Content: conversation.Content{
		conversation.ToolUseBlock{ID: "call-1", Name: "shell", Input: []byte(`{"commands"}`)},
	}}
	for _, event := range []agentloop.Event{
		{
			Type: agentloop.EventModelStream, SessionID: sessionID, RoundID: roundID,
			Stream: &modelcall.ModelCallStreamEvent{
				Type:      modelcall.ModelCallStreamEventToolUseDone,
				ToolInput: []byte(`{"commands"}`),
			},
		},
		{
			Type: agentloop.EventModelStream, SessionID: sessionID, RoundID: roundID,
			Stream: &modelcall.ModelCallStreamEvent{
				Type:     modelcall.ModelCallStreamEventCompleted,
				Response: invalidResponse,
			},
		},
		{Type: agentloop.EventRoundEnded, SessionID: sessionID, RoundID: roundID, Status: conversation.RoundCompleted},
	} {
		event := event
		if err := notifier.OnEvent(t.Context(), &middleware.EventContext{Event: &event}); err != nil {
			t.Fatal(err)
		}
	}

	if len(sent) != 3 {
		t.Fatalf("notifications = %d, want 3", len(sent))
	}
	if sent[0].Sequence != 1 || sent[0].Stream == nil || sent[0].Stream.ToolInput != nil {
		t.Fatalf("tool completion event = %#v", sent[0])
	}
	if sent[1].Sequence != 2 || sent[1].Stream == nil || sent[1].Stream.Response == nil {
		t.Fatalf("model completion event = %#v", sent[1])
	}
	completedCall := sent[1].Stream.Response.Content[0].(conversation.ToolUseBlock)
	if string(completedCall.Input) != `{}` {
		t.Fatalf("completed tool input = %q", completedCall.Input)
	}
	if sent[2].Sequence != 3 {
		t.Fatalf("round completion event = %#v", sent[2])
	}
}

func TestSessionNotificationMiddlewareProjectsToolReviewEvents(t *testing.T) {
	sessionID := uuid.Must(uuid.NewV7())
	roundID := uuid.Must(uuid.NewV7())
	review := permission.ReviewEvent{ToolUseID: "call-review"}
	var sent []SessionEvent
	notifier := NewSessionNotificationMiddleware(func(_ context.Context, _ string, payload any) error {
		sent = append(sent, payload.(SessionEvent))
		return nil
	})

	for _, eventType := range []agentloop.EventType{
		permission.EventReviewStarted,
		permission.EventReviewResolved,
	} {
		event := agentloop.Event{
			Type:      eventType,
			SessionID: sessionID,
			RoundID:   roundID,
			Iteration: 2,
			Payload:   review,
		}
		if err := notifier.OnEvent(t.Context(), &middleware.EventContext{Event: &event}); err != nil {
			t.Fatal(err)
		}
	}

	if len(sent) != 2 {
		t.Fatalf("review notifications = %d, want 2", len(sent))
	}
	for index, event := range sent {
		if event.Sequence != uint64(index+1) || event.Iteration != 2 || event.Review == nil || *event.Review != review {
			t.Fatalf("review notification %d = %#v", index, event)
		}
	}
}

func TestApprovalMessageSurvivesNotificationProjection(t *testing.T) {
	request := permission.Request{ApprovalID: uuid.New(), Message: "Confirm discarding uncommitted changes."}
	notifier := NewSessionNotificationMiddleware(func(_ context.Context, _ string, payload any) error {
		event, ok := payload.(SessionEvent)
		if !ok || event.Approval == nil || event.Approval.Message != request.Message {
			t.Fatalf("approval reason was lost: %#v", payload)
		}
		return nil
	})
	event := agentloop.Event{Type: permission.EventRequested, SessionID: uuid.New(), RoundID: uuid.New(), Payload: request}
	if err := notifier.OnEvent(t.Context(), &middleware.EventContext{Event: &event}); err != nil {
		t.Fatal(err)
	}
}

func TestSessionNotificationMiddlewareProjectsCodexMode(t *testing.T) {
	sessionID := uuid.Must(uuid.NewV7())
	notifier := NewSessionNotificationMiddleware(func(_ context.Context, _ string, payload any) error {
		event, ok := payload.(SessionEvent)
		if !ok {
			t.Fatalf("payload = %T, want SessionEvent", payload)
		}
		if event.Type != SessionEventToolDialectChanged || event.ToolDialect != conversation.ToolDialectCodex {
			t.Fatalf("event = %#v", event)
		}
		return nil
	})
	event := agentloop.Event{
		Type:      agentloop.EventToolDialectChanged,
		SessionID: sessionID,
		Payload:   conversation.SessionCodexModeEnabled{SessionID: sessionID},
	}
	if err := notifier.OnEvent(t.Context(), &middleware.EventContext{Event: &event}); err != nil {
		t.Fatal(err)
	}
}
