package session_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	inframiddleware "github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
	infrasession "github.com/masteryyh/agenty-core/pkg/infra/session"
	infrastorage "github.com/masteryyh/agenty-core/pkg/infra/storage"
)

func TestEngineRunsLifecycleAndLoopHooksAtTheExpectedBoundaries(t *testing.T) {
	fixture := newExecutionFixture(t, 8_192)
	caller := &scriptedCaller{responses: []*modelcall.ModelCallResponse{
		{Content: conversation.Text("first"), StopReason: modelcall.ModelCallStopReasonEndTurn},
		{Content: conversation.Text("second"), StopReason: modelcall.ModelCallStopReasonEndTurn},
	}}
	var events []string
	engine, err := infrasession.NewEngine(t.Context(), infrasession.Dependencies{
		Sessions:    fixture.sessions,
		Catalog:     fixture.catalog,
		Tools:       fixture.registry,
		InvokeModel: caller.Call,
		LoopHooks: agentloop.LoopHooks{
			BeforeModelCall: func(_ context.Context, state *agentloop.ModelCallState) error {
				events = append(events, "before-model")
				state.Request.SystemPrompt += "|hook"
				return nil
			},
			AfterModelCall: func(_ context.Context, _ *agentloop.ModelCallState) error {
				events = append(events, "after-model")
				return nil
			},
		},
		Lifecycle: inframiddleware.LifecycleHooks{
			BeforeSessionStart: func(_ context.Context, state *inframiddleware.SessionStartContext) error {
				events = append(events, "before-session")
				*state.SystemPrompt += "|session"
				return nil
			},
			BeforeRound: func(_ context.Context, state *inframiddleware.RoundContext) error {
				events = append(events, "before-round")
				*state.SystemPrompt += "|round"
				if state.Round != nil || state.RoundID != uuid.Nil {
					t.Errorf("BeforeRound received an allocated round: %#v", state.Round)
				}
				return nil
			},
			AfterRound: func(_ context.Context, state *inframiddleware.RoundContext) error {
				events = append(events, "after-round")
				if !state.Status.Terminal() {
					t.Errorf("AfterRound status = %q", state.Status)
				}
				return nil
			},
			AfterSessionStop: func(_ context.Context, state *inframiddleware.SessionStopContext) error {
				events = append(events, "after-session")
				state.Session.SetTitle("stopped")
				return nil
			},
			OnEvent: infrastorage.NewSessionMiddleware(fixture.sessions).OnEvent,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	session := fixture.createSession(t)
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = engine.Shutdown(shutdownCtx)
	})

	if _, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("one")); err != nil {
		t.Fatal(err)
	}
	waitForExecution(t, engine, session.ID)
	if _, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("two")); err != nil {
		t.Fatal(err)
	}
	waitForExecution(t, engine, session.ID)
	if err := engine.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	persisted, err := fixture.sessions.Load(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Summary().Title != "stopped" {
		t.Fatalf("after-session mutation was not persisted: %+v", persisted.Summary())
	}
	for index, request := range caller.Requests() {
		if !strings.Contains(request.SystemPrompt, "|session") {
			t.Fatalf("request %d lost session-level system prompt mutation: %q", index, request.SystemPrompt)
		}
		if strings.Count(request.SystemPrompt, "|round") != 1 {
			t.Fatalf("request %d accumulated per-round prompt mutations: %q", index, request.SystemPrompt)
		}
	}

	want := []string{
		"before-session", "before-round", "before-model", "after-model", "after-round",
		"before-round", "before-model", "after-model", "after-round", "after-session",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("hook events = %v, want %v", events, want)
	}
}
