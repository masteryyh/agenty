package permission

import (
	"context"
	"errors"
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
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
)

func TestReviewResultValidation(t *testing.T) {
	tests := []struct {
		name, text string
		want       ReviewResult
		invalid    bool
	}{
		{name: "allow omits reason", text: `{"decision":"allow"}`, want: ReviewResult{Decision: Allow}},
		{name: "allow null", text: `{"decision":"allow","message":null}`, want: ReviewResult{Decision: Allow}},
		{name: "ask", text: `{"decision":"ask","message":" Confirm deleting local changes. "}`, want: ReviewResult{Decision: Ask, Message: "Confirm deleting local changes."}},
		{name: "deny", text: `{"decision":"deny","message":"This would disclose credentials."}`, want: ReviewResult{Decision: Deny, Message: "This would disclose credentials."}},
		{name: "missing message", text: `{"decision":"ask"}`, invalid: true},
		{name: "null message", text: `{"decision":"deny","message":null}`, invalid: true},
		{name: "empty message", text: `{"decision":"deny","message":""}`, invalid: true},
		{name: "blank message", text: `{"decision":"ask","message":"  "}`, invalid: true},
		{name: "wrong message type", text: `{"decision":"ask","message":false}`, invalid: true},
		{name: "wrong decision type", text: `{"decision":true,"message":null}`, invalid: true},
		{name: "allow message", text: `{"decision":"allow","message":"reason"}`, invalid: true},
		{name: "allow empty message", text: `{"decision":"allow","message":""}`, invalid: true},
		{name: "missing decision", text: `{"message":null}`, invalid: true},
		{name: "null decision", text: `{"decision":null}`, invalid: true},
		{name: "duplicate decision", text: `{"decision":"deny","decision":"allow","message":null}`, invalid: true},
		{name: "duplicate escaped key", text: `{"decision":"allow","decisi\u006fn":"allow","message":null}`, invalid: true},
		{name: "duplicate message", text: `{"decision":"ask","message":null,"message":"reason"}`, invalid: true},
		{name: "extra field", text: `{"decision":"allow","message":null,"x":1}`, invalid: true},
		{name: "trailing object", text: `{"decision":"allow"}{}`, invalid: true},
		{name: "array", text: `[{"decision":"allow"}]`, invalid: true},
		{name: "null", text: `null`, invalid: true},
		{name: "markdown", text: "```json\n{\"decision\":\"allow\"}\n```", invalid: true},
		{name: "newline", text: `{"decision":"ask","message":"Reason\nmore"}`, invalid: true},
		{name: "terminal escape", text: `{"decision":"ask","message":"\u001b[31mred"}`, invalid: true},
		{name: "bidi", text: `{"decision":"ask","message":"\u202eevil"}`, invalid: true},
		{name: "long message", text: `{"decision":"ask","message":"` + strings.Repeat("x", 241) + `"}`, invalid: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := reviewDecision(&modelcall.ModelCallResponse{
				Content: conversation.Text(test.text), StopReason: modelcall.ModelCallStopReasonEndTurn,
			})
			if test.invalid {
				if err == nil {
					t.Fatalf("accepted %#v", result)
				}
				return
			}
			if err != nil || result != test.want {
				t.Fatalf("result = %#v, %v; want %#v", result, err, test.want)
			}
		})
	}
}

func TestReviewParsesCompleteTextBeforeCheckingStopReason(t *testing.T) {
	content := conversation.Content{
		conversation.ReasoningBlock{Reasoning: "internal reasoning"},
		conversation.TextBlock{Text: `{"decision":"allow",`},
		conversation.TextBlock{Text: `"message":null}`},
	}
	for _, reason := range []modelcall.ModelCallStopReason{
		modelcall.ModelCallStopReasonEndTurn, modelcall.ModelCallStopReasonMaxTokens,
		modelcall.ModelCallStopReasonContentFilter, modelcall.ModelCallStopReasonError,
		modelcall.ModelCallStopReasonToolUse, "",
	} {
		_, err := reviewDecision(&modelcall.ModelCallResponse{Content: content, StopReason: reason})
		complete := reason == modelcall.ModelCallStopReasonEndTurn ||
			reason == modelcall.ModelCallStopReasonMaxTokens || reason == ""
		if (err == nil) != complete {
			t.Fatalf("stop reason %q: %v", reason, err)
		}
		_, err = reviewDecision(&modelcall.ModelCallResponse{
			Content:    conversation.Text(`{"decision":"allow","message":`),
			StopReason: reason,
		})
		if err == nil {
			t.Fatalf("accepted truncated JSON with stop reason %q", reason)
		}
	}
	if _, err := reviewDecision(nil); err == nil {
		t.Fatal("accepted nil response")
	}
}

func TestReviewSelectsLowestSupportedReasoning(t *testing.T) {
	tests := []struct {
		name      string
		supported bool
		levels    []shared.ReasoningEffort
		want      shared.ReasoningEffort
		invalid   bool
	}{
		{name: "non reasoning", levels: []shared.ReasoningEffort{shared.ReasoningLow}},
		{name: "unsorted", supported: true, levels: []shared.ReasoningEffort{shared.ReasoningHigh, shared.ReasoningLow, shared.ReasoningMedium}, want: shared.ReasoningLow},
		{name: "medium minimum", supported: true, levels: []shared.ReasoningEffort{shared.ReasoningMax, shared.ReasoningOff, shared.ReasoningMedium}, want: shared.ReasoningMedium},
		{name: "missing levels", supported: true, invalid: true},
		{name: "off only", supported: true, levels: []shared.ReasoningEffort{shared.ReasoningOff}, invalid: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := reviewReasoningEffort(modelcall.ModelCallConfig{SupportsReasoning: test.supported, ReasoningEfforts: test.levels})
			if (err != nil) != test.invalid || got != test.want {
				t.Fatalf("effort = %q, %v", got, err)
			}
		})
	}
}

func TestAutoReviewRetriesInvalidOutputsAndTransientRequests(t *testing.T) {
	for _, scenario := range []string{
		"valid ask", "invalid then allow", "invalid exhausted", "transient", "unauthorized",
		"unsupported schema", "truncated then allow", "complete at token limit", "combined retries",
	} {
		t.Run(scenario, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := attempts.Add(1)
				data, _ := io.ReadAll(r.Body)
				var body map[string]any
				if err := json.Unmarshal(data, &body); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if body["max_completion_tokens"] != float64(512) || body["reasoning_effort"] != "medium" {
					t.Errorf("review settings = %v", body)
				}
				format, ok := body["response_format"].(map[string]any)
				if !ok || format["type"] != "json_schema" {
					t.Errorf("missing output format: %v", body)
				}
				if scenario == "unauthorized" {
					w.WriteHeader(401)
					fmt.Fprint(w, `{"error":{"message":"Unauthorized"}}`)
					return
				}
				if scenario == "unsupported schema" {
					w.WriteHeader(400)
					fmt.Fprint(w, `{"error":{"message":"Unsupported response_format"}}`)
					return
				}
				if (scenario == "transient" && n < 3) || (scenario == "combined retries" && n%3 != 0) {
					w.Header().Set("Retry-After", "0.001")
					w.WriteHeader(503)
					return
				}
				answer := `{"decision":"allow","message":null}`
				finish := "stop"
				if scenario == "valid ask" {
					answer = `{"decision":"ask","message":"Confirm discarding uncommitted changes."}`
				}
				if scenario == "invalid exhausted" || (scenario == "invalid then allow" && n == 1) || (scenario == "combined retries" && n == 3) {
					answer = `{"decision":"ask"}`
				}
				if scenario == "truncated then allow" && n == 1 {
					answer = `{"decision":"allow","message":`
					finish = "length"
				}
				if scenario == "complete at token limit" {
					finish = "length"
				}
				if n == 2 && (scenario == "invalid then allow" || scenario == "invalid exhausted" || scenario == "truncated then allow") {
					if !strings.Contains(string(data), "failed validation") {
						t.Error("missing correction feedback")
					}
				}
				encoded, _ := json.Marshal(answer)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"id":"review","choices":[{"message":{"role":"assistant","content":%s},"finish_reason":%q}]}`, encoded, finish)
			}))
			defer server.Close()
			model := modelcall.ModelCallConfig{BaseURL: server.URL, APIType: modelcall.APIOpenAICompletions, APIKey: "test", ModelCode: "review", SupportsReasoning: true, ReasoningEfforts: []shared.ReasoningEffort{shared.ReasoningHigh, shared.ReasoningMedium}}
			result, err := autoReview(t.Context(), model, "Review git diff in cwd.")
			wantAttempts := int32(1)
			switch scenario {
			case "invalid then allow", "invalid exhausted", "truncated then allow":
				wantAttempts = 2
			case "transient":
				wantAttempts = 3
			case "combined retries":
				wantAttempts = 6
			}
			if attempts.Load() != wantAttempts {
				t.Fatalf("attempts = %d; want %d", attempts.Load(), wantAttempts)
			}
			switch scenario {
			case "invalid exhausted":
				if err != nil || result.Decision != Deny || result.Message != "Auto review failed: invalid output after retry." {
					t.Fatalf("result = %#v, error = %v", result, err)
				}
			case "unauthorized", "unsupported schema":
				if err == nil {
					t.Fatal("expected request error")
				}
			case "valid ask":
				if err != nil || result.Decision != Ask || result.Message == "" {
					t.Fatalf("result = %#v, %v", result, err)
				}
			default:
				if err != nil || result.Decision != Allow {
					t.Fatalf("result = %#v, %v", result, err)
				}
			}
		})
	}
}

func TestAutoReviewCancellationStopsRetries(t *testing.T) {
	var attempts atomic.Int32
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		close(started)
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(503)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := autoReview(ctx, modelcall.ModelCallConfig{APIType: modelcall.APIOpenAICompletions, APIKey: "test", ModelCode: "review", BaseURL: server.URL}, "review")
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not stop retry")
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d", attempts.Load())
	}
}
