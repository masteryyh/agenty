//go:build e2e

package e2e_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type chatCompletionRequest struct {
	Model               string `json:"model"`
	MaxCompletionTokens int64  `json:"max_completion_tokens"`
	Messages            []any  `json:"messages"`
}

func TestSessionExecutionCompletesParallelRounds(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	requests := make(chan chatCompletionRequest, 2)
	var arrived atomic.Int32
	var releaseOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Errorf("request path = %q, want /v1/chat/completions", request.URL.Path)
			http.NotFound(writer, request)
			return
		}

		var payload chatCompletionRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode chat completion request: %v", err)
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}
		requests <- payload
		if arrived.Add(1) == 2 {
			releaseOnce.Do(func() {
				close(release)
			})
		}

		select {
		case <-release:
		case <-request.Context().Done():
			return
		}

		writer.Header().Set("Content-Type", "application/json")
		if _, err := writer.Write([]byte(`{
			"id":"chatcmpl-e2e",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-e2e",
			"choices":[{"index":0,"message":{"role":"assistant","content":"completed"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}
		}`)); err != nil {
			t.Errorf("write chat completion response: %v", err)
		}
	}))
	defer server.Close()

	core := startCore(t)
	configureExecutionResources(t, core, server.URL+"/v1")
	first := createSession(t, core, "runner", "openai-e2e", "gpt-e2e")
	second := createSession(t, core, "runner", "openai-e2e", "gpt-e2e")

	firstStart := startSessionRound(t, core, first.ID, "first")
	secondStart := startSessionRound(t, core, second.ID, "second")
	if firstStart.Status != "running" || secondStart.Status != "running" {
		t.Fatalf("start results = %+v, %+v", firstStart, secondStart)
	}

	firstCompleted := waitForSessionRoundStatus(t, core, first.ID, "completed")
	secondCompleted := waitForSessionRoundStatus(t, core, second.ID, "completed")
	for _, session := range []sessionView{firstCompleted, secondCompleted} {
		round := session.Rounds[0]
		if len(round.Messages) != 2 || round.Messages[1].Role != "assistant" {
			t.Fatalf("completed round messages = %+v", round.Messages)
		}
		if len(round.Messages[1].Content) != 1 || round.Messages[1].Content[0].Text != "completed" {
			t.Fatalf("assistant content = %+v", round.Messages[1].Content)
		}
		if round.Usage != (tokenUsageView{Input: 2, Output: 3, Total: 5}) {
			t.Fatalf("round usage = %+v", round.Usage)
		}
	}

	for range 2 {
		select {
		case payload := <-requests:
			if payload.Model != "gpt-e2e" || payload.MaxCompletionTokens != 65_536 {
				t.Fatalf("LLM request = %+v", payload)
			}
			if len(payload.Messages) < 2 {
				t.Fatalf("LLM messages = %d, want system and user messages", len(payload.Messages))
			}
		case <-time.After(processTimeout):
			t.Fatal("did not receive both LLM requests")
		}
	}
}

func TestSessionExecutionStopCancelsRound(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() {
		close(release)
	})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		startedOnce.Do(func() {
			close(started)
		})
		select {
		case <-request.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()

	core := startCore(t)
	session := createExecutionSession(t, core, server.URL+"/v1")
	startResult := startSessionRound(t, core, session.ID, "wait")

	select {
	case <-started:
	case <-time.After(processTimeout):
		t.Fatal("LLM request did not start")
	}

	requireRPCError(t, core.Call(t, "session.start", map[string]any{
		"id":      session.ID,
		"content": []map[string]any{{"type": "text", "text": "again"}},
	}), errAlreadyExists)
	stopResult := decodeResult[executionStopView](t, core.Call(t, "session.stop", map[string]any{
		"id": session.ID,
	}))
	if stopResult.SessionID != session.ID || stopResult.RoundID != startResult.RoundID || !stopResult.StopRequested {
		t.Fatalf("stop result = %+v", stopResult)
	}

	cancelledSession := waitForSessionRoundStatus(t, core, session.ID, "cancelled")
	if cancelledSession.Rounds[0].Error != nil {
		t.Fatalf("cancelled round error = %q", *cancelledSession.Rounds[0].Error)
	}
	releaseOnce.Do(func() {
		close(release)
	})
	requireRPCError(t, core.Call(t, "session.stop", map[string]any{"id": session.ID}), errNotFound)
}

func createExecutionSession(t *testing.T, core *coreProcess, baseURL string) sessionView {
	t.Helper()
	configureExecutionResources(t, core, baseURL)

	return createSession(t, core, "runner", "openai-e2e", "gpt-e2e")
}

func configureExecutionResources(t *testing.T, core *coreProcess, baseURL string) {
	t.Helper()

	requireSuccess(t, core.Call(t, "agent.create", map[string]any{
		"slug": "runner",
		"name": "Runner",
		"soul": "Complete the user request.",
	}))
	requireSuccess(t, core.Call(t, "provider.create", map[string]any{
		"slug":    "openai-e2e",
		"name":    "OpenAI E2E",
		"type":    "openai_completions",
		"baseUrl": baseURL,
		"apiKey":  "test-key",
	}))
	requireSuccess(t, core.Call(t, "provider.addModel", map[string]any{
		"providerSlug":    "openai-e2e",
		"modelSlug":       "gpt-e2e",
		"name":            "GPT E2E",
		"contextWindow":   128000,
		"maxOutputTokens": 100000,
	}))
}

func startSessionRound(
	t *testing.T,
	core *coreProcess,
	sessionID string,
	text string,
) executionStartView {
	t.Helper()

	return decodeResult[executionStartView](t, core.Call(t, "session.start", map[string]any{
		"id": sessionID,
		"content": []map[string]any{{
			"type": "text",
			"text": text,
		}},
	}))
}

func waitForSessionRoundStatus(
	t *testing.T,
	core *coreProcess,
	sessionID string,
	want string,
) sessionView {
	t.Helper()

	deadline := time.Now().Add(processTimeout)
	for {
		session := decodeResult[sessionView](t, core.Call(t, "session.get", map[string]any{"id": sessionID}))
		if len(session.Rounds) == 1 && session.Rounds[0].Status == want {
			return session
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %s did not reach round status %q: %+v", sessionID, want, session.Rounds)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
