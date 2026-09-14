package compaction_test

import (
	"context"
	"strings"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/compaction"
	"github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
)

func TestMiddlewareCompactsBeforeModelCallAndAccountsForUsage(t *testing.T) {
	request := modelcall.ModelCallRequest{
		SystemPrompt: strings.Repeat("prompt ", 30),
		Messages: []modelcall.ModelCallMessage{{
			Role:    conversation.RoleUser,
			Content: conversation.Text(strings.Repeat("message ", 30)),
		}},
	}
	attempted := false
	state := &middleware.ModelCallContext{
		Request: &request,
	}
	state.Context = compaction.WithOperations(t.Context(), compaction.Operations{
		ContextWindow:   100,
		MaxOutputTokens: 10,
		Attempted:       &attempted,
		Compact: func(context.Context) (conversation.TokenUsage, error) {
			return conversation.TokenUsage{Input: 4, Output: 2, Total: 6}, nil
		},
		RebuildRequest: func(context.Context) (modelcall.ModelCallRequest, error) {
			return modelcall.ModelCallRequest{SystemPrompt: "rebuilt"}, nil
		},
	})

	hook := compaction.NewMiddleware().BeforeModelCall
	if err := hook(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	if state.Request.SystemPrompt != "rebuilt" {
		t.Fatalf("request was not rebuilt: %+v", state.Request)
	}
	if !attempted {
		t.Fatal("compaction attempt was not recorded")
	}
	if state.RequestUsage.Total != 6 {
		t.Fatalf("request usage = %+v, want total 6", state.RequestUsage)
	}

	if err := hook(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	if state.RequestUsage.Total != 6 {
		t.Fatalf("compaction ran more than once: %+v", state.RequestUsage)
	}
}
