package session_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
	"github.com/masteryyh/agenty-core/pkg/infra/permission"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
	infrasession "github.com/masteryyh/agenty-core/pkg/infra/session"
	"github.com/masteryyh/agenty-core/pkg/infra/storage"
	infratools "github.com/masteryyh/agenty-core/pkg/infra/tools"
)

func TestEngineHITLLifecycle(t *testing.T) {
	for _, action := range []string{"allow", "deny", "yolo", "stop", "shutdown"} {
		t.Run(action, func(t *testing.T) {
			fixture := newExecutionFixture(t, 8192)
			session := fixture.createSession(t)
			var executed atomic.Int32
			if err := fixture.registry.Register(&executionTestTool{
				definition: modelcall.ToolDefinition{Name: "lookup"},
				execute: func(context.Context, agentloop.CallContext, []byte) (conversation.Content, error) {
					executed.Add(1)
					return conversation.Text("found"), nil
				},
			}); err != nil {
				t.Fatal(err)
			}
			caller := &scriptedCaller{responses: []*modelcall.ModelCallResponse{
				{Content: conversation.Content{conversation.ToolUseBlock{ID: "lookup-1", Name: "lookup", Input: []byte(`{"key":"value"}`)}}},
				{Content: conversation.Text("done")},
			}}
			manager := permission.NewPermissionManager()
			middlewares := middleware.NewManager()
			events := make(chan rpc.SessionEvent, 64)
			for _, mw := range []middleware.Middleware{
				infratools.NewValidationMiddleware(),
				storage.NewSessionMiddleware(fixture.sessions),
				rpc.NewSessionNotificationMiddleware(func(_ context.Context, _ string, payload any) error {
					events <- payload.(rpc.SessionEvent)
					return nil
				}),
				manager.Middleware(),
			} {
				if err := middlewares.Register(mw); err != nil {
					t.Fatal(err)
				}
			}
			chain, err := middlewares.Compile()
			if err != nil {
				t.Fatal(err)
			}
			engine, err := infrasession.NewEngine(t.Context(), infrasession.Dependencies{
				Sessions: fixture.sessions, Catalog: fixture.catalog, Tools: fixture.registry,
				InvokeModel: caller.Call, LoopHooks: chain.AgentLoopHooks(), Lifecycle: chain.LifecycleHooks(),
				PermissionModeChanged: manager.PermissionModeChanged,
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := engine.Shutdown(ctx); err != nil {
					t.Error(err)
				}
			})
			start, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("lookup"))
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			var sequence uint64
			next := func() rpc.SessionEvent {
				select {
				case event := <-events:
					sequence++
					if event.Sequence != sequence || event.SessionID != session.ID || event.RoundID != start.RoundID {
						t.Fatalf("invalid event identity/sequence: %+v", event)
					}
					return event
				case <-ctx.Done():
					t.Fatal("timed out waiting for execution event")
					return rpc.SessionEvent{}
				}
			}
			var request *permission.Request
			for request == nil {
				event := next()
				request = event.Approval
			}
			if executed.Load() != 0 || len(caller.Requests()) != 1 {
				t.Fatal("execution advanced before approval")
			}
			if request.ToolCall.ID != "lookup-1" || request.Preview.Title != "Agenty wants to call this tool: lookup" {
				t.Fatalf("generic preview = %+v", request)
			}
			resolution := permission.Resolution{SessionID: session.ID, RoundID: start.RoundID, ApprovalID: request.ApprovalID, Decision: permission.Allow}
			switch action {
			case "allow", "deny":
				resolution.Decision = permission.Decision(action)
				err = manager.Resolve(ctx, resolution)
			case "yolo":
				_, err = engine.SetPermissionMode(ctx, session.ID.String(), conversation.PermissionYolo)
			case "stop":
				_, err = engine.Stop(ctx, session.ID.String())
			case "shutdown":
				err = engine.Shutdown(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			var ended rpc.SessionEvent
			for ended.Type != rpc.SessionEventRoundEnded {
				ended = next()
			}
			if manager.Resolve(ctx, resolution) == nil {
				t.Fatal("stale approval survived completion")
			}
			if action == "stop" || action == "shutdown" {
				if executed.Load() != 0 || ended.Status != conversation.RoundCancelled {
					t.Fatalf("cancelled execution: %d, %s", executed.Load(), ended.Status)
				}
				return
			}
			if ended.Status != conversation.RoundCompleted {
				t.Fatalf("round ended: %+v", ended)
			}
			if (executed.Load() == 1) != (action == "allow" || action == "yolo") {
				t.Fatalf("executions = %d", executed.Load())
			}
			persisted, err := fixture.sessions.Load(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			var result *conversation.ToolResultBlock
			for _, message := range persisted.Rounds[0].Messages {
				for _, block := range message.Content {
					if toolResult, ok := block.(conversation.ToolResultBlock); ok {
						result = &toolResult
					}
				}
			}
			if result == nil || result.ToolUseID != "lookup-1" || result.IsError != (action == "deny") {
				t.Fatalf("persisted result = %+v", result)
			}
			requests := caller.Requests()
			if len(requests) != 2 {
				t.Fatalf("model calls = %d", len(requests))
			}
			found := false
			for _, message := range requests[1].Messages {
				for _, block := range message.Content {
					if toolResult, ok := block.(conversation.ToolResultBlock); ok && toolResult.ToolUseID == "lookup-1" {
						found = true
						if action == "deny" && toolResult.Content[0].(conversation.TextBlock).Text != permission.DeniedMessage {
							t.Fatal("model did not receive denial")
						}
					}
				}
			}
			if !found {
				t.Fatal("continuation omitted tool result")
			}
		})
	}
}
