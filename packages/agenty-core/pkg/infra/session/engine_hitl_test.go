package session_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	json "github.com/bytedance/sonic"

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
				if err == nil {
					if manager.Resolve(ctx, resolution) != nil {
						t.Fatal("pending mode must leave the current approval available")
					}
				}
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
			for index, message := range requests[1].Messages {
				if message.Role != conversation.RoleAssistant {
					continue
				}
				if index+1 >= len(requests[1].Messages) {
					t.Fatal("assistant tool call has no following result message")
				}
				next := requests[1].Messages[index+1]
				if len(next.Content) != 1 {
					t.Fatalf("message after tool call = %+v", next)
				}
				if _, ok := next.Content[0].(conversation.ToolResultBlock); !ok {
					t.Fatal("permission metadata interrupted the tool call/result sequence")
				}
			}
		})
	}
}

func TestEnginePermissionSwitchesPreserveResponsesToolSequence(t *testing.T) {
	t.Parallel()

	fixture := newExecutionFixture(t, 8192)
	session := fixture.createSession(t)
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var executions atomic.Int32
	if err := fixture.registry.Register(&executionTestTool{
		definition: modelcall.ToolDefinition{Name: "lookup", InputSchema: modelcall.JSONSchema{Type: "object"}},
		execute: func(ctx context.Context, _ agentloop.CallContext, _ []byte) (conversation.Content, error) {
			executions.Add(1)
			started <- struct{}{}
			select {
			case <-release:
				return conversation.Text("found"), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}); err != nil {
		t.Fatal(err)
	}

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		var wire struct {
			Input []struct {
				Type    string `json:"type"`
				CallID  string `json:"call_id"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"input"`
		}
		if err := json.Unmarshal(body, &wire); err != nil {
			t.Error(err)
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}

		number := requests.Add(1)
		output := []map[string]any{}
		if number == 1 {
			for _, id := range []string{"lookup-1", "lookup-2"} {
				output = append(output, map[string]any{
					"type": "function_call", "id": "fc_" + id, "call_id": id,
					"name": "lookup", "arguments": `{}`, "status": "completed",
				})
			}
		} else {
			pending := map[string]bool{}
			results := 0
			updates := 0
			for _, item := range wire.Input {
				switch item.Type {
				case "function_call":
					pending[item.CallID] = true
				case "function_call_output":
					if !pending[item.CallID] {
						t.Errorf("unmatched tool output %s", item.CallID)
					}
					delete(pending, item.CallID)
					results++
				default:
					if len(pending) > 0 {
						t.Errorf("request %d interrupted pending calls %v with %s", number, pending, item.Type)
						http.Error(writer, "No tool output found", http.StatusBadRequest)
						return
					}
					for _, content := range item.Content {
						if results == 2 && strings.Contains(content.Text, "<permission-mode>") {
							updates++
						}
					}
				}
			}
			if len(pending) != 0 || results != 2 || updates < 1 {
				t.Errorf("request %d: pending=%v, results=%d, permission updates=%d", number, pending, results, updates)
			}
			output = append(output, map[string]any{
				"type": "message", "id": "msg_done", "role": "assistant", "status": "completed",
				"content": []map[string]any{{"type": "output_text", "text": "done"}},
			})
		}
		encoded, err := json.Marshal(map[string]any{
			"type": "response.completed", "sequence_number": 1,
			"response": map[string]any{
				"id": fmt.Sprintf("resp_%d", number), "model": "gpt-5", "status": "completed", "output": output,
				"usage": map[string]int{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
			},
		})
		if err != nil {
			t.Error(err)
			return
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(writer, "event: response.completed\ndata: %s\n\n", encoded)
	}))
	t.Cleanup(server.Close)
	invoke := func(
		ctx context.Context,
		model modelcall.ModelCallConfig,
		request modelcall.ModelCallRequest,
		handler modelcall.ModelCallStreamHandler,
	) (*modelcall.ModelCallResponse, error) {
		model.BaseURL = server.URL
		return modelcall.Call(ctx, model, request, modelcall.WithMaxRetries(0), modelcall.WithStreamHandler(handler))
	}
	engine := fixture.newEngine(t, invoke)
	if _, err := engine.Start(t.Context(), session.ID.String(), conversation.Text("look up two items")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("tools did not start")
	}
	for _, mode := range []conversation.PermissionMode{
		conversation.PermissionYolo, conversation.PermissionAsk, conversation.PermissionAuto, conversation.PermissionYolo,
	} {
		updated, err := engine.SetPermissionMode(t.Context(), session.ID.String(), mode)
		if err != nil {
			t.Fatal(err)
		}
		if updated.CurrentPermissionMode() != conversation.PermissionAsk {
			t.Fatalf("permission mode changed before the next model call: %s", updated.CurrentPermissionMode())
		}
		wantPending := mode
		if mode == conversation.PermissionAsk {
			wantPending = ""
		}
		if engine.PendingPermissionMode(session.ID) != wantPending {
			t.Fatalf("pending mode = %s, want %s", engine.PendingPermissionMode(session.ID), wantPending)
		}
	}
	close(release)
	waitForExecution(t, engine, session.ID)
	loaded, err := fixture.sessions.Load(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Rounds[0].Status != conversation.RoundCompleted {
		t.Fatalf("first round failed: %+v", loaded.Rounds[0].Error)
	}
	changes := 0
	for _, event := range fixture.sessions.events[session.ID] {
		if change, ok := event.(conversation.SessionPermissionModeChanged); ok {
			changes++
			if change.PermissionMode != conversation.PermissionYolo {
				t.Fatalf("persisted an intermediate mode: %s", change.PermissionMode)
			}
		}
	}
	if changes != 1 {
		t.Fatalf("permission change events = %d, want 1", changes)
	}
	if err := engine.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Reload the persisted history in a new engine without executing the tools again.
	restarted := fixture.newEngine(t, invoke)
	if _, err := restarted.Start(t.Context(), session.ID.String(), conversation.Text("continue")); err != nil {
		t.Fatal(err)
	}
	waitForExecution(t, restarted, session.ID)
	loaded, err = fixture.sessions.Load(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Rounds[1].Status != conversation.RoundCompleted || requests.Load() != 3 || executions.Load() != 2 {
		t.Fatalf("resumed status=%s, requests=%d, tool executions=%d", loaded.Rounds[1].Status, requests.Load(), executions.Load())
	}
}
