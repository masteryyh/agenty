package conversation

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestSessionConfigurationAndRoundSnapshots(t *testing.T) {
	t.Parallel()

	model1 := shared.NewModelRef("anthropic", "claude-opus-4")
	model2 := shared.NewModelRef("anthropic", "claude-haiku-4")
	cwd1 := "/workspace/one"
	cwd2 := "/workspace/two"

	session := StartSession("coder", model1, 200_000, shared.ThinkingHigh, &cwd1)
	cwd1 = "/mutated/by/caller"
	round1, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}

	session.SetModel(model2, 128_000)
	session.SetThinkingEffort(shared.ThinkingLow)
	session.SetCwd(&cwd2)
	cwd2 = "/also/mutated"
	round2, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}

	if got := session.Rounds[0]; got.ID != round1 || got.Model != model1 || got.Cwd == nil || *got.Cwd != "/workspace/one" {
		t.Errorf("first round snapshot = %+v", got)
	}
	if got := session.Rounds[1]; got.ID != round2 || got.Model != model2 || got.Cwd == nil || *got.Cwd != "/workspace/two" {
		t.Errorf("second round snapshot = %+v", got)
	}

	replayed := ReplaySession(session.PendingEvents())
	if replayed.CurrentModel == nil || *replayed.CurrentModel != model2 || replayed.ContextWindow != 128_000 {
		t.Errorf("replayed model configuration = %+v, %d", replayed.CurrentModel, replayed.ContextWindow)
	}
	if replayed.Cwd == nil || *replayed.Cwd != "/workspace/two" || replayed.CurrentThinkingEffort != shared.ThinkingLow {
		t.Errorf("replayed execution configuration = cwd %v, thinking %q", replayed.Cwd, replayed.CurrentThinkingEffort)
	}
}

func TestSessionLifecycleAndReplay(t *testing.T) {
	t.Parallel()

	model := shared.NewModelRef("anthropic", "claude-opus-4")
	session := StartSession("coder", model, 200_000, shared.ThinkingHigh, nil)
	roundID, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendUserMessage(roundID, Text("hello")); err != nil {
		t.Fatal(err)
	}
	usage := &TokenUsage{Input: 10, Output: 20, Total: 30}
	if _, err := session.AppendAssistantMessage(roundID, Text("hi there"), model, usage); err != nil {
		t.Fatal(err)
	}
	if err := session.CompleteRound(roundID, RoundCompleted, *usage, nil); err != nil {
		t.Fatal(err)
	}
	session.SetTitle("greeting")

	replayed := ReplaySession(session.PendingEvents())
	if replayed.ID != session.ID || len(replayed.Rounds) != 1 {
		t.Fatalf("replayed session = %+v", replayed)
	}
	round := replayed.Rounds[0]
	if round.Status != RoundCompleted || len(round.Messages) != 2 || round.EndedAt == nil {
		t.Errorf("replayed round = %+v", round)
	}
	if round.Messages[1].Usage == nil || *round.Messages[1].Usage != *usage || round.Messages[1].Model == nil || *round.Messages[1].Model != model {
		t.Errorf("assistant message metadata = %+v", round.Messages[1])
	}
	summary := replayed.Summary()
	if summary.Title != "greeting" || summary.LastProviderSlug != "anthropic" || summary.LastModelSlug != "claude-opus-4" {
		t.Errorf("summary = %+v", summary)
	}
}

func TestSessionRejectsInvalidTransitions(t *testing.T) {
	t.Parallel()

	model := shared.NewModelRef("anthropic", "claude-opus-4")
	tests := []struct {
		name string
		call func(*Session, uuid.UUID) error
		want error
	}{
		{name: "invalid role", call: func(s *Session, id uuid.UUID) error {
			_, err := s.AppendMessage(id, "tool", Text("x"), nil, nil)
			return err
		}, want: ErrInvalidRole},
		{name: "append unknown round", call: func(s *Session, _ uuid.UUID) error {
			_, err := s.AppendUserMessage(shared.NewID(), Text("x"))
			return err
		}, want: ErrRoundNotFound},
		{name: "complete unknown round", call: func(s *Session, _ uuid.UUID) error {
			return s.CompleteRound(shared.NewID(), RoundCompleted, TokenUsage{}, nil)
		}, want: ErrRoundNotFound},
		{name: "append completed round", call: func(s *Session, id uuid.UUID) error {
			if err := s.CompleteRound(id, RoundCompleted, TokenUsage{}, nil); err != nil {
				return err
			}
			_, err := s.AppendUserMessage(id, Text("x"))
			return err
		}, want: ErrRoundNotRunning},
		{name: "complete twice", call: func(s *Session, id uuid.UUID) error {
			if err := s.CompleteRound(id, RoundCompleted, TokenUsage{}, nil); err != nil {
				return err
			}
			return s.CompleteRound(id, RoundCompleted, TokenUsage{}, nil)
		}, want: ErrRoundNotRunning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			session := StartSession("coder", model, 200_000, shared.ThinkingOff, nil)
			roundID, err := session.StartRound()
			if err != nil {
				t.Fatal(err)
			}
			if err := tt.call(session, roundID); !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSessionCompleteRoundTerminalStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status RoundStatus
		errMsg *string
	}{
		{name: "completed", status: RoundCompleted},
		{name: "failed", status: RoundFailed, errMsg: stringPointer("provider unavailable")},
		{name: "cancelled", status: RoundCancelled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			session := StartSession("coder", shared.NewModelRef("anthropic", "claude-opus"), 200_000, shared.ThinkingOff, nil)
			roundID, err := session.StartRound()
			if err != nil {
				t.Fatal(err)
			}
			if err := session.CompleteRound(roundID, tt.status, TokenUsage{Total: 1}, tt.errMsg); err != nil {
				t.Fatal(err)
			}
			round := session.Rounds[0]
			if round.Status != tt.status || round.EndedAt == nil {
				t.Errorf("round = %+v", round)
			}
			if tt.errMsg != nil && (round.Error == nil || *round.Error != *tt.errMsg) {
				t.Errorf("round error = %v, want %q", round.Error, *tt.errMsg)
			}
		})
	}
}

func TestSessionRequiresConfiguredModelAndTerminalStatus(t *testing.T) {
	t.Parallel()

	modelTests := []struct {
		name    string
		session *Session
	}{
		{name: "nil model", session: &Session{}},
		{name: "zero model", session: StartSession("coder", shared.ModelRef{}, 0, shared.ThinkingOff, nil)},
	}
	for _, tt := range modelTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := tt.session.StartRound(); !errors.Is(err, ErrModelNotConfigured) {
				t.Errorf("StartRound error = %v, want ErrModelNotConfigured", err)
			}
		})
	}

	session := StartSession("coder", shared.NewModelRef("anthropic", "claude-opus"), 0, shared.ThinkingOff, nil)
	roundID, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	if err := session.CompleteRound(roundID, RoundRunning, TokenUsage{}, nil); err == nil {
		t.Error("CompleteRound accepted a non-terminal status")
	}
}

func TestSessionClearPending(t *testing.T) {
	t.Parallel()

	session := StartSession("coder", shared.NewModelRef("anthropic", "claude-opus"), 0, shared.ThinkingOff, nil)
	if len(session.PendingEvents()) != 1 {
		t.Fatalf("pending events = %d, want 1", len(session.PendingEvents()))
	}
	session.ClearPending()
	if len(session.PendingEvents()) != 0 {
		t.Errorf("pending events = %d, want 0", len(session.PendingEvents()))
	}
	if session.ID == uuid.Nil {
		t.Error("ClearPending changed projected session state")
	}
}

func stringPointer(value string) *string { return &value }
