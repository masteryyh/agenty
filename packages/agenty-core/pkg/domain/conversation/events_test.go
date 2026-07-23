package conversation

import (
	"reflect"
	"testing"
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestEventEnvelopeRoundTrip(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	sessionID := shared.NewID()
	roundID := shared.NewID()
	model := shared.NewModelRef("anthropic", "claude-opus-4")
	cwd := "/workspace"
	errMessage := "provider unavailable"

	tests := []struct {
		name  string
		event shared.Event
	}{
		{name: "session started", event: SessionStarted{SessionID: sessionID, Agent: "coder", Model: model, ContextWindow: 200_000, ThinkingEffort: shared.ThinkingHigh, Cwd: &cwd, At: at}},
		{name: "model set", event: SessionModelSet{SessionID: sessionID, Model: model, ContextWindow: 200_000, At: at}},
		{name: "thinking effort set", event: SessionThinkingEffortSet{SessionID: sessionID, ThinkingEffort: shared.ThinkingHigh, At: at}},
		{name: "cwd cleared", event: SessionCwdSet{SessionID: sessionID, Cwd: nil, At: at}},
		{name: "round started", event: RoundStarted{SessionID: sessionID, RoundID: roundID, Sequence: 1, Model: model, ContextWindow: 200_000, ThinkingEffort: shared.ThinkingHigh, Cwd: &cwd, At: at}},
		{name: "message appended", event: MessageAppended{SessionID: sessionID, Message: Message{ID: shared.NewID(), RoundID: roundID, Role: RoleAssistant, Content: Text("hi"), Model: &model, Usage: &TokenUsage{Input: 10, Output: 20, Total: 30}, CreatedAt: at}, At: at}},
		{name: "round failed", event: RoundEnded{SessionID: sessionID, RoundID: roundID, Status: RoundFailed, Usage: TokenUsage{Input: 10, Output: 20, Total: 30}, Error: &errMessage, At: at}},
		{name: "title set", event: SessionTitleSet{SessionID: sessionID, Title: "greeting", At: at}},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			line, err := shared.EncodeEvent(int64(i+1), tt.event)
			if err != nil {
				t.Fatalf("EncodeEvent: %v", err)
			}
			decoded, err := DecodeEventLine(line)
			if err != nil {
				t.Fatalf("DecodeEventLine: %v", err)
			}
			if !reflect.DeepEqual(decoded, tt.event) {
				t.Errorf("decoded event = %#v, want %#v", decoded, tt.event)
			}
		})
	}
}

func TestDecodeEventLineRejectsInvalidPersistenceData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		line string
	}{
		{name: "malformed envelope", line: `{bad json`},
		{name: "unknown event type", line: `{"type":"unknown","seq":1,"payload":{},"wroteAt":"2026-07-20T10:00:00Z"}`},
		{name: "malformed payload", line: `{"type":"session_started","seq":1,"payload":"bad","wroteAt":"2026-07-20T10:00:00Z"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeEventLine([]byte(tt.line)); err == nil {
				t.Error("DecodeEventLine succeeded, want error")
			}
		})
	}
}
