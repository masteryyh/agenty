package conversation

import (
	"testing"
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestEventEnvelopeRoundTrip(t *testing.T) {
	at := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	sid := shared.NewID()
	rid := shared.NewID()
	ref := shared.NewModelRef("anthropic", "claude-opus-4")
	cwd := "/workspace"

	events := []shared.Event{
		SessionStarted{SessionID: sid, Agent: "coder", Model: ref, ContextWindow: 200_000, ThinkingEffort: shared.ThinkingHigh, Cwd: &cwd, At: at},
		SessionModelSet{SessionID: sid, Model: ref, ContextWindow: 200_000, At: at},
		SessionThinkingEffortSet{SessionID: sid, ThinkingEffort: shared.ThinkingHigh, At: at},
		SessionCwdSet{SessionID: sid, Cwd: nil, At: at},
		RoundStarted{SessionID: sid, RoundID: rid, Sequence: 1, Model: ref, ContextWindow: 200_000, ThinkingEffort: shared.ThinkingHigh, Cwd: &cwd, At: at},
		MessageAppended{SessionID: sid, Message: Message{ID: shared.NewID(), RoundID: rid, Role: RoleUser, Content: Text("hi"), CreatedAt: at}, At: at},
		RoundEnded{SessionID: sid, RoundID: rid, Status: RoundCompleted, Usage: TokenUsage{Input: 10, Output: 20, Total: 30}, At: at},
		SessionTitleSet{SessionID: sid, Title: "greeting", At: at},
	}

	for i, e := range events {
		line, err := shared.EncodeEvent(int64(i+1), e)
		if err != nil {
			t.Fatalf("encode %s: %v", e.EventType(), err)
		}
		decoded, err := DecodeEventLine(line)
		if err != nil {
			t.Fatalf("decode %s: %v", e.EventType(), err)
		}
		if decoded.EventType() != e.EventType() {
			t.Errorf("type mismatch: got %q, want %q", decoded.EventType(), e.EventType())
		}
		if !decoded.OccurredAt().Equal(e.OccurredAt()) {
			t.Errorf("%s time mismatch: got %v, want %v", e.EventType(), decoded.OccurredAt(), e.OccurredAt())
		}
	}
}

func TestSessionConfigurationAndRoundSnapshots(t *testing.T) {
	model1 := shared.NewModelRef("anthropic", "claude-opus-4")
	model2 := shared.NewModelRef("anthropic", "claude-haiku-4")
	cwd1 := "/workspace/one"
	cwd2 := "/workspace/two"

	s := StartSession("coder", model1, 200_000, shared.ThinkingHigh, &cwd1)
	round1, err := s.StartRound()
	if err != nil {
		t.Fatalf("start first round: %v", err)
	}

	s.SetModel(model2, 128_000)
	s.SetThinkingEffort(shared.ThinkingLow)
	s.SetCwd(&cwd2)
	round2, err := s.StartRound()
	if err != nil {
		t.Fatalf("start second round: %v", err)
	}

	if got := s.Rounds[0]; got.ID != round1 || got.Model != model1 || got.Cwd == nil || *got.Cwd != cwd1 {
		t.Errorf("first round snapshot = %+v, want initial configuration", got)
	}
	if got := s.Rounds[1]; got.ID != round2 || got.Model != model2 || got.Cwd == nil || *got.Cwd != cwd2 {
		t.Errorf("second round snapshot = %+v, want updated configuration", got)
	}

	// Replaying the pending events must reconstruct equivalent current configuration
	// and immutable round snapshots.
	replayed := ReplaySession(s.PendingEvents())
	if replayed.CurrentModel == nil || *replayed.CurrentModel != model2 {
		t.Errorf("current model = %v, want %v", replayed.CurrentModel, model2)
	}
	if replayed.ContextWindow != 128_000 {
		t.Errorf("context window = %d, want 128000", replayed.ContextWindow)
	}
	if replayed.CurrentThinkingEffort != shared.ThinkingLow {
		t.Errorf("thinking effort = %q, want low", replayed.CurrentThinkingEffort)
	}
	if len(replayed.Rounds) != 2 || replayed.Rounds[0].Model != model1 || replayed.Rounds[1].Model != model2 {
		t.Errorf("round snapshots = %+v, want both configurations", replayed.Rounds)
	}

	s.SetCwd(nil)
	if s.Cwd != nil {
		t.Errorf("cwd = %v, want nil", s.Cwd)
	}
}

func TestSessionLifecycleAndReplay(t *testing.T) {
	ref := shared.NewModelRef("anthropic", "claude-opus-4")

	s := StartSession("coder", ref, 200_000, shared.ThinkingHigh, nil)
	round, err := s.StartRound()
	if err != nil {
		t.Fatalf("start round: %v", err)
	}

	if _, err := s.AppendUserMessage(round, Text("hello")); err != nil {
		t.Fatalf("append user message: %v", err)
	}
	usage := &TokenUsage{Input: 10, Output: 20, Total: 30}
	if _, err := s.AppendAssistantMessage(round, Text("hi there"), ref, usage); err != nil {
		t.Fatalf("append assistant message: %v", err)
	}
	if err := s.CompleteRound(round, RoundCompleted, TokenUsage{Input: 10, Output: 20, Total: 30}, nil); err != nil {
		t.Fatalf("complete round: %v", err)
	}
	s.SetTitle("greeting")

	replayed := ReplaySession(s.PendingEvents())
	if replayed.ID != s.ID {
		t.Errorf("id mismatch: got %v, want %v", replayed.ID, s.ID)
	}
	if replayed.AgentSlug != "coder" {
		t.Errorf("agent mismatch: got %q", replayed.AgentSlug)
	}
	if len(replayed.Rounds) != 1 {
		t.Fatalf("round count = %d, want 1", len(replayed.Rounds))
	}
	r := replayed.Rounds[0]
	if r.Status != RoundCompleted {
		t.Errorf("round status = %q, want completed", r.Status)
	}
	if len(r.Messages) != 2 {
		t.Errorf("message count = %d, want 2", len(r.Messages))
	}
	if r.Sequence != 1 {
		t.Errorf("round sequence = %d, want 1", r.Sequence)
	}
	if r.EndedAt == nil {
		t.Error("expected round EndedAt to be set")
	}

	sum := replayed.Summary()
	if sum.Title != "greeting" {
		t.Errorf("summary title = %q, want greeting", sum.Title)
	}
	if sum.LastProviderSlug != "anthropic" || sum.LastModelSlug != "claude-opus-4" {
		t.Errorf("summary model ref = %s/%s", sum.LastProviderSlug, sum.LastModelSlug)
	}
	if sum.ContextWindow != 200_000 {
		t.Errorf("summary context window = %d, want 200000", sum.ContextWindow)
	}
	if sum.LastThinkingEffort != shared.ThinkingHigh {
		t.Errorf("summary thinking effort = %q, want high", sum.LastThinkingEffort)
	}
}

func TestStartRoundRequiresModel(t *testing.T) {
	s := &Session{}
	if _, err := s.StartRound(); err != ErrModelNotConfigured {
		t.Errorf("expected ErrModelNotConfigured, got %v", err)
	}
}

func TestAppendMessageToUnknownRound(t *testing.T) {
	ref := shared.NewModelRef("anthropic", "claude-opus-4")
	s := StartSession("coder", ref, 200_000, shared.ThinkingOff, nil)
	if _, err := s.AppendUserMessage(shared.NewID(), Text("hi")); err != ErrRoundNotFound {
		t.Errorf("expected ErrRoundNotFound, got %v", err)
	}
}

func TestCompleteRoundRejectsNonTerminalStatus(t *testing.T) {
	ref := shared.NewModelRef("anthropic", "claude-opus-4")
	s := StartSession("coder", ref, 200_000, shared.ThinkingOff, nil)
	round, err := s.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteRound(round, RoundRunning, TokenUsage{}, nil); err == nil {
		t.Error("expected error for non-terminal status, got nil")
	}
}
