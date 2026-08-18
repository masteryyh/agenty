package conversation

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestSessionEmptyRoundsEncodeAsArray(t *testing.T) {
	t.Parallel()

	session := StartSession(
		"coder",
		shared.NewModelRef("anthropic", "claude-opus-4"),
		200_000,
		shared.ReasoningOff,
		nil,
	)

	encoded, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if got := string(payload["rounds"]); got != "[]" {
		t.Errorf("rounds = %s, want []", got)
	}

	replayed := ReplaySession(roundTripEvents(t, session.PendingEvents()))
	replayedEncoded, err := json.Marshal(replayed)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(replayedEncoded, &payload); err != nil {
		t.Fatal(err)
	}
	if got := string(payload["rounds"]); got != "[]" {
		t.Errorf("replayed rounds = %s, want []", got)
	}
}

func TestSessionConfigurationAndRoundSnapshots(t *testing.T) {
	t.Parallel()

	model1 := shared.NewModelRef("anthropic", "claude-opus-4")
	model2 := shared.NewModelRef("anthropic", "claude-haiku-4")
	cwd1 := "/workspace/one"
	cwd2 := "/workspace/two"

	session := StartSession("coder", model1, 200_000, shared.ReasoningHigh, &cwd1)
	cwd1 = "/mutated/by/caller"
	round1, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}

	session.SetModel(model2, 128_000)
	session.SetReasoningEffort(shared.ReasoningLow)
	session.SetCwd(&cwd2)
	cwd2 = "/also/mutated"
	round2, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}

	if got := session.Rounds[0]; got.ID != round1 || got.Model != model1 || got.ContextWindow != 200_000 || got.ReasoningEffort != shared.ReasoningHigh || got.Cwd == nil || *got.Cwd != "/workspace/one" {
		t.Errorf("first round snapshot = %+v", got)
	}
	if got := session.Rounds[1]; got.ID != round2 || got.Model != model2 || got.ContextWindow != 128_000 || got.ReasoningEffort != shared.ReasoningLow || got.Cwd == nil || *got.Cwd != "/workspace/two" {
		t.Errorf("second round snapshot = %+v", got)
	}

	replayed := ReplaySession(roundTripEvents(t, session.PendingEvents()))
	if replayed.CurrentModel == nil || *replayed.CurrentModel != model2 || replayed.ContextWindow != 128_000 {
		t.Errorf("replayed model configuration = %+v, %d", replayed.CurrentModel, replayed.ContextWindow)
	}
	if replayed.Cwd == nil || *replayed.Cwd != "/workspace/two" || replayed.CurrentReasoningEffort != shared.ReasoningLow {
		t.Errorf("replayed execution configuration = cwd %v, reasoning %q", replayed.Cwd, replayed.CurrentReasoningEffort)
	}
}

func TestSessionLifecycleAndReplay(t *testing.T) {
	t.Parallel()

	model := shared.NewModelRef("anthropic", "claude-opus-4")
	session := StartSession("coder", model, 200_000, shared.ReasoningHigh, nil)
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
	if round.Usage != *usage {
		t.Errorf("round usage = %+v, want %+v", round.Usage, *usage)
	}
	if round.Messages[1].Usage == nil || *round.Messages[1].Usage != *usage || round.Messages[1].Model == nil || *round.Messages[1].Model != model {
		t.Errorf("assistant message metadata = %+v", round.Messages[1])
	}
	summary := replayed.Summary()
	if summary.Title != "greeting" || summary.LastProviderSlug != "anthropic" || summary.LastModelSlug != "claude-opus-4" {
		t.Errorf("summary = %+v", summary)
	}
}

func TestSessionCompactionReplacesOnlyEffectiveContext(t *testing.T) {
	t.Parallel()

	model := shared.NewModelRef("anthropic", "claude-opus")
	session := StartSession("coder", model, 200_000, shared.ReasoningOff, nil)
	roundID, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendUserMessage(roundID, Text("goal")); err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendAssistantMessage(roundID, Text("implemented"), model, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendHiddenUserMessage(roundID, Text("<metadata><model>claude-opus</model></metadata>")); err != nil {
		t.Fatal(err)
	}
	rawRounds := len(session.Rounds[0].Messages)
	if _, err := session.Compact(CompactionInput{
		Trigger: CompactionTriggerManual,
		Summary: "Task goals: goal\nCompleted: implemented",
	}); err != nil {
		t.Fatal(err)
	}

	if len(session.Rounds[0].Messages) != rawRounds {
		t.Fatalf("raw round messages = %d, want %d", len(session.Rounds[0].Messages), rawRounds)
	}
	context := session.ContextMessages()
	if len(context) != 4 || context[0].Role != RoleUser || !context[1].IsHidden() || !context[2].IsHidden() || context[3].Role != RoleAssistant {
		t.Fatalf("effective context = %+v", context)
	}
	if got, _ := context[0].Content[0].(TextBlock); got.Text != "goal" {
		t.Errorf("retained user context = %+v", context[0])
	}
	if kind, _ := context[1].Metadata["compactionKind"].(string); kind != "summary" {
		t.Errorf("summary context kind = %q", kind)
	}
	if kind, _ := context[2].Metadata["compactionKind"].(string); kind != "metadata" {
		t.Errorf("metadata context kind = %q", kind)
	}

	replayed := ReplaySession(roundTripEvents(t, session.PendingEvents()))
	if len(replayed.Rounds[0].Messages) != rawRounds || len(replayed.ContextMessages()) != 4 {
		t.Fatalf("replayed rounds/context = %d/%d", len(replayed.Rounds[0].Messages), len(replayed.ContextMessages()))
	}
	if replayed.ContextMessages()[0].Role != RoleUser || replayed.ContextMessages()[1].Metadata["compactionKind"] != "summary" {
		t.Fatalf("replayed context order = %+v", replayed.ContextMessages())
	}
}

func TestSessionCompactionMetadataTracksModelChangesInPlace(t *testing.T) {
	t.Parallel()

	model := shared.NewModelRef("anthropic", "claude-opus")
	session := StartSession("coder", model, 200_000, shared.ReasoningOff, nil)
	roundID, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendUserMessage(roundID, Text("goal")); err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendHiddenUserMessage(roundID, Text("<metadata><model>old</model></metadata>")); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Compact(CompactionInput{Trigger: CompactionTriggerModelSwitch, Summary: "summary"}); err != nil {
		t.Fatal(err)
	}
	session.SetModel(shared.NewModelRef("anthropic", "new"), 200_000)

	context := session.ContextMessages()
	if len(context) != 3 || context[0].Role != RoleUser || context[1].Metadata["compactionKind"] != "summary" || context[2].Metadata["compactionKind"] != "metadata" {
		t.Fatalf("context = %+v, want user, summary, metadata", context)
	}
	if got := context[2].Content[0].(TextBlock).Text; !strings.Contains(got, "<model>new</model>") {
		t.Errorf("refreshed metadata = %q", got)
	}
	metadata := session.LastMetadata()
	if metadata == nil || metadata.Model != "new" {
		t.Fatalf("cached metadata = %+v", metadata)
	}

	replayed := ReplaySession(roundTripEvents(t, session.PendingEvents()))
	if got := replayed.ContextMessages()[2].Content[0].(TextBlock).Text; !strings.Contains(got, "<model>new</model>") {
		t.Errorf("replayed metadata = %q", got)
	}
}

func TestSessionCompactionRetainsThreeUsersBeforeSummaryAndFiveAssistantsAfter(t *testing.T) {
	t.Parallel()

	model := shared.NewModelRef("anthropic", "claude-opus")
	session := StartSession("coder", model, 200_000, shared.ReasoningOff, nil)
	roundID, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendHiddenUserMessage(roundID, Text("<metadata><model>claude-opus</model></metadata>")); err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 6; index++ {
		if _, err := session.AppendUserMessage(roundID, Text(fmt.Sprintf("user-%d", index))); err != nil {
			t.Fatal(err)
		}
		assistantContent := Text(fmt.Sprintf("assistant-%d", index))
		if index == 6 {
			assistantContent = Content{
				ReasoningBlock{Reasoning: "private"},
				ToolUseBlock{ID: "lookup", Name: "read_file", Input: shared.RawJSON(`{"path":"README.md"}`)},
				TextBlock{Text: "assistant-6"},
			}
		}
		if _, err := session.AppendAssistantMessage(roundID, assistantContent, model, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := session.AppendUserMessage(roundID, Content{
		ToolResultBlock{ToolUseID: "lookup", Content: Text("tool output")},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := session.Compact(CompactionInput{Trigger: CompactionTriggerManual, Summary: "summary"}); err != nil {
		t.Fatal(err)
	}
	context := session.ContextMessages()
	if len(context) != 10 {
		t.Fatalf("context length = %d, want 10", len(context))
	}
	for index := 0; index < 3; index++ {
		message := context[index]
		if message.Role != RoleUser || message.Content[0].(TextBlock).Text != fmt.Sprintf("user-%d", index+4) {
			t.Errorf("retained user[%d] = %+v", index, message)
		}
	}
	if context[3].Metadata["compactionKind"] != "summary" || context[4].Metadata["compactionKind"] != "metadata" {
		t.Fatalf("synthetic context order = %+v", context[3:5])
	}
	for index := 0; index < 5; index++ {
		message := context[index+5]
		if message.Role != RoleAssistant || message.Content[0].(TextBlock).Text != fmt.Sprintf("assistant-%d", index+2) {
			t.Errorf("retained assistant[%d] = %+v", index, message)
		}
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
			session := StartSession("coder", model, 200_000, shared.ReasoningOff, nil)
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

func roundTripEvents(t *testing.T, events []shared.Event) []shared.Event {
	t.Helper()

	decoded := make([]shared.Event, 0, len(events))
	for index, event := range events {
		line, err := shared.EncodeEvent(int64(index+1), event)
		if err != nil {
			t.Fatalf("encode event %d: %v", index, err)
		}
		replayed, err := DecodeEventLine(line)
		if err != nil {
			t.Fatalf("decode event %d: %v", index, err)
		}
		decoded = append(decoded, replayed)
	}
	return decoded
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
			session := StartSession("coder", shared.NewModelRef("anthropic", "claude-opus"), 200_000, shared.ReasoningOff, nil)
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
		{name: "zero model", session: StartSession("coder", shared.ModelRef{}, 0, shared.ReasoningOff, nil)},
	}
	for _, tt := range modelTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := tt.session.StartRound(); !errors.Is(err, ErrModelNotConfigured) {
				t.Errorf("StartRound error = %v, want ErrModelNotConfigured", err)
			}
		})
	}

	session := StartSession("coder", shared.NewModelRef("anthropic", "claude-opus"), 0, shared.ReasoningOff, nil)
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

	session := StartSession("coder", shared.NewModelRef("anthropic", "claude-opus"), 0, shared.ReasoningOff, nil)
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
