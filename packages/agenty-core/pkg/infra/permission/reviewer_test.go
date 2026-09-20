package permission

import (
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
)

func TestReviewDecisionAcceptsOnlyDecisionObject(t *testing.T) {
	tests := []struct {
		name    string
		content conversation.Content
		want    Decision
		wantErr bool
	}{
		{
			name:    "allow",
			content: conversation.Text(`{"decision":"allow","message":null}`),
			want:    Allow,
		},
		{
			name:    "deny",
			content: conversation.Text(`{"decision":"deny","message":"reason"}`),
			want:    Deny,
		},
		{
			name:    "ask",
			content: conversation.Text(`{"decision":"ask","message":"reason"}`),
			want:    Ask,
		},
		{
			name:    "additional property",
			content: conversation.Text(`{"decision":"allow","message":null,"reason":"too much output"}`),
			wantErr: true,
		},
		{
			name:    "invalid value",
			content: conversation.Text(`{"decision":"yes"}`),
			wantErr: true,
		},
		{
			name:    "non text response",
			content: conversation.Content{conversation.ToolUseBlock{ID: "call", Name: "tool", Input: []byte(`{}`)}},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := reviewDecision(&modelcall.ModelCallResponse{Content: test.content, StopReason: modelcall.ModelCallStopReasonEndTurn})
			if test.wantErr {
				if err == nil {
					t.Fatal("reviewDecision() succeeded")
				}
				return
			}
			if err != nil || got.Decision != test.want {
				t.Fatalf("reviewDecision() = %#v, %v; want %q, nil", got, err, test.want)
			}
		})
	}
}
