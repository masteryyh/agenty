/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package chat

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/providers"
	"github.com/masteryyh/agenty/pkg/tools"
)

const testAPIType models.APIType = "test-session-hooks"

type hookTestProvider struct {
	chatRequests   []*providers.ChatRequest
	streamResponse *providers.Message
}

func (p *hookTestProvider) Chat(_ context.Context, req *providers.ChatRequest) (*providers.ChatResponse, error) {
	p.chatRequests = append(p.chatRequests, req)
	if len(req.Messages) > 0 {
		last := req.Messages[len(req.Messages)-1]
		if last.ToolResult != nil {
			return &providers.ChatResponse{Content: last.ToolResult.Content, TotalToken: 3}, nil
		}
	}
	return &providers.ChatResponse{
		Content: "tool requested",
		ToolCalls: []models.ToolCall{
			{ID: "call_1", Name: "blocked", Arguments: `{}`},
		},
		TotalToken: 2,
	}, nil
}

func (p *hookTestProvider) StreamChat(ctx context.Context, _ *providers.ChatRequest) (<-chan providers.StreamEvent, error) {
	ch := make(chan providers.StreamEvent, 2)
	go func() {
		defer close(ch)
		select {
		case ch <- providers.StreamEvent{Type: providers.EventMessageDone, Message: p.streamResponse}:
		case <-ctx.Done():
			return
		}
		select {
		case ch <- providers.StreamEvent{Type: providers.EventUsage, Usage: &providers.StreamUsage{TotalTokens: 1}}:
		case <-ctx.Done():
		}
	}()
	return ch, nil
}

func (p *hookTestProvider) Embed(context.Context, *providers.EmbeddingRequest) (*providers.EmbeddingResponse, error) {
	return &providers.EmbeddingResponse{}, nil
}

func (p *hookTestProvider) Name() string {
	return "hook-test"
}

func (p *hookTestProvider) VectorNormalized() bool {
	return false
}

type hookTestTool struct{}

func (t *hookTestTool) Definition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Name:        "echo",
		Description: "test tool",
		Parameters: tools.ToolParameters{
			Type:       "object",
			Properties: map[string]tools.ParameterProperty{},
		},
	}
}

func (t *hookTestTool) Execute(_ context.Context, _ tools.ToolCallContext, arguments string) (string, error) {
	return arguments, nil
}

func TestChatExecutorHooksCanMutateRequestMessageToolCallAndResult(t *testing.T) {
	resetSessionHooksForTest()
	defer resetSessionHooksForTest()

	provider := &hookTestProvider{}
	previousProvider, hadProvider := providers.ModelProviders[testAPIType]
	providers.ModelProviders[testAPIType] = provider
	defer func() {
		if hadProvider {
			providers.ModelProviders[testAPIType] = previousProvider
		} else {
			delete(providers.ModelProviders, testAPIType)
		}
	}()

	RegisterSessionHook(SessionHookBeforeModelCall, "request", SessionHookOptions{Async: true}, func(_ context.Context, hookCtx *SessionHookContext) error {
		time.Sleep(10 * time.Millisecond)
		hookCtx.Request.Model = "hooked-model"
		return nil
	})
	RegisterSessionHook(SessionHookAfterModelResponse, "message", SessionHookOptions{}, func(_ context.Context, hookCtx *SessionHookContext) error {
		if len(hookCtx.Message.ToolCalls) > 0 {
			hookCtx.Message.Content = "hooked assistant"
		}
		return nil
	})
	RegisterSessionHook(SessionHookBeforeToolExecution, "tool-call", SessionHookOptions{}, func(_ context.Context, hookCtx *SessionHookContext) error {
		hookCtx.ToolCall.Name = "echo"
		hookCtx.ToolCall.Arguments = "hooked arguments"
		return nil
	})
	RegisterSessionHook(SessionHookAfterToolExecution, "tool-result", SessionHookOptions{}, func(_ context.Context, hookCtx *SessionHookContext) error {
		hookCtx.ToolResult.Content = "hooked result"
		return nil
	})

	registry := tools.GetRegistry()
	registry.Register(&hookTestTool{})
	defer registry.Unregister("echo")

	executor := NewChatExecutor(registry)
	result, err := executor.Chat(context.Background(), &ChatParams{
		Model:   "original-model",
		APIType: testAPIType,
		Messages: []providers.Message{
			{Role: models.RoleUser, Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if len(provider.chatRequests) != 2 {
		t.Fatalf("chat request count = %d", len(provider.chatRequests))
	}
	for _, req := range provider.chatRequests {
		if req.Model != "hooked-model" {
			t.Fatalf("request model = %q", req.Model)
		}
	}
	if len(result.Messages) != 3 {
		t.Fatalf("message count = %d", len(result.Messages))
	}
	if result.Messages[0].Content != "hooked assistant" {
		t.Fatalf("assistant content = %q", result.Messages[0].Content)
	}
	if len(result.Messages[0].ToolCalls) != 1 || result.Messages[0].ToolCalls[0].Name != "echo" || result.Messages[0].ToolCalls[0].Arguments != "hooked arguments" {
		t.Fatalf("assistant tool calls = %#v", result.Messages[0].ToolCalls)
	}
	if result.Messages[1].ToolResult == nil || result.Messages[1].ToolResult.Content != "hooked result" {
		t.Fatalf("tool result = %#v", result.Messages[1].ToolResult)
	}
	if result.Messages[2].Content != "hooked result" {
		t.Fatalf("final assistant content = %q", result.Messages[2].Content)
	}
}

func TestStreamChatRunsAfterModelResponseBeforeMessageDone(t *testing.T) {
	resetSessionHooksForTest()
	defer resetSessionHooksForTest()

	provider := &hookTestProvider{
		streamResponse: &providers.Message{Role: models.RoleAssistant, Content: "raw"},
	}
	previousProvider, hadProvider := providers.ModelProviders[testAPIType]
	providers.ModelProviders[testAPIType] = provider
	defer func() {
		if hadProvider {
			providers.ModelProviders[testAPIType] = previousProvider
		} else {
			delete(providers.ModelProviders, testAPIType)
		}
	}()

	var calls []string
	RegisterSessionHook(SessionHookAfterModelResponse, "stream-message", SessionHookOptions{}, func(_ context.Context, hookCtx *SessionHookContext) error {
		calls = append(calls, "hook")
		hookCtx.Message.Content = "hooked"
		return nil
	})

	executor := NewChatExecutor(tools.GetRegistry())
	ch, err := executor.StreamChat(context.Background(), &ChatParams{
		Model:   "stream-model",
		APIType: testAPIType,
		Messages: []providers.Message{
			{Role: models.RoleUser, Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}

	var messageDone *providers.Message
	for evt := range ch {
		if evt.Type == providers.EventMessageDone {
			messageDone = evt.Message
			calls = append(calls, "messageDone")
		}
	}

	if messageDone == nil {
		t.Fatal("missing message done event")
	}
	if messageDone.Content != "hooked" {
		t.Fatalf("message content = %q", messageDone.Content)
	}
	if !slices.Equal(calls, []string{"hook", "messageDone"}) {
		t.Fatalf("calls = %#v", calls)
	}
}
