package rpc

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
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
