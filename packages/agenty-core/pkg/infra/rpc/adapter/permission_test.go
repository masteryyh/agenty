package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/middleware"
	permission "github.com/masteryyh/agenty-core/pkg/infra/permission"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
)

func TestResolveToolApprovalRejectsInvalidInput(t *testing.T) {
	for _, tt := range []struct {
		name, params string
		code         int
	}{
		{"empty", `{}`, rpc.ErrCodeInvalidParams},
		{"invalid uuid", `{"sessionId":"bad"}`, rpc.ErrCodeInvalidParams},
		{"missing decision", `{"sessionId":"00000000-0000-4000-8000-000000000001","roundId":"00000000-0000-4000-8000-000000000002","approvalId":"00000000-0000-4000-8000-000000000003"}`, rpc.ErrCodeInvalidParams},
		{"stale", `{"sessionId":"00000000-0000-4000-8000-000000000001","roundId":"00000000-0000-4000-8000-000000000002","approvalId":"00000000-0000-4000-8000-000000000003","decision":"allow"}`, rpc.ErrCodeNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveToolApproval(permission.NewPermissionManager())(t.Context(), json.RawMessage(tt.params))
			var rpcErr *rpc.Error
			if !errors.As(err, &rpcErr) || rpcErr.Code != tt.code {
				t.Fatalf("error = %v, want code %d", err, tt.code)
			}
		})
	}
}

func TestResolveToolApprovalOverStdio(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		manager := permission.NewPermissionManager()
		state := &middleware.ToolCallContext{
			Session: &conversation.Session{ID: uuid.New()}, Round: &conversation.Round{ID: uuid.New()},
			Call: &conversation.ToolUseBlock{ID: "call-1", Name: "lookup", Input: []byte(`{}`)},
		}
		var approvalID uuid.UUID
		state.Emit = func(_ context.Context, event agentloop.Event) error {
			if request, ok := event.Payload.(permission.Request); ok {
				approvalID = request.ApprovalID
			}
			return nil
		}
		done := make(chan error, 1)
		go func() { done <- manager.Middleware().BeforeToolCall(t.Context(), state) }()
		synctest.Wait()
		input, err := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "session.resolveToolApproval",
			"params": permission.Resolution{SessionID: state.Session.ID, RoundID: state.Round.ID, ApprovalID: approvalID, Decision: permission.Deny},
		})
		if err != nil {
			t.Fatal(err)
		}
		dispatcher := rpc.NewDispatcher()
		RegisterHITLHandlers(dispatcher, manager)
		var output bytes.Buffer
		server := rpc.NewServer(dispatcher, bytes.NewReader(append(input, '\n')), &output)
		if err := server.Serve(t.Context()); err != nil {
			t.Fatal(err)
		}
		var response struct {
			ID     int                   `json:"id"`
			Result permission.Resolution `json:"result"`
			Error  *rpc.Error            `json:"error"`
		}
		if err := json.Unmarshal(output.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Error != nil || response.ID != 1 || response.Result.ApprovalID != approvalID || response.Result.Decision != permission.Deny {
			t.Fatalf("response = %+v", response)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if state.Result == nil || state.Result.ToolUseID != "call-1" || !state.Result.IsError {
			t.Fatalf("tool result = %+v", state.Result)
		}
	})
}
