package conversation

import (
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestContextMessagesKeepsToolResultsBeforeMetadata(t *testing.T) {
	t.Parallel()

	roundID := uuid.New()
	otherRoundID := uuid.New()
	messages := map[string]Message{
		"calls": {Role: RoleAssistant, Content: Content{
			ToolUseBlock{ID: "read", Name: "read_file", Input: []byte(`{}`)},
			ShellCallBlock{CallID: "shell", Commands: []string{"pwd"}},
			ApplyPatchCallBlock{CallID: "patch", Source: ApplyPatchSourceCustom, Patch: "patch"},
		}},
		"auto": {
			Role: RoleDeveloper, Visibility: MessageHidden,
			Metadata: shared.Metadata{"kind": "metadata", "scope": "round"},
			Content:  Text("<metadata><permission-mode>auto</permission-mode></metadata>"),
		},
		"yolo": {
			Role: RoleUser, Visibility: MessageHidden,
			Metadata: shared.Metadata{"kind": "metadata", "scope": "round"},
			Content:  Text("<metadata><permission-mode>yolo</permission-mode></metadata>"),
		},
		"read": {Role: RoleUser, Content: Content{
			ToolResultBlock{ToolUseID: "read", Content: Text("file contents")},
		}},
		"rest": {Role: RoleUser, Content: Content{
			ToolResultBlock{ToolUseID: "shell", Content: Text("/workspace")},
			ToolResultBlock{ToolUseID: "patch", IsError: true, Content: Text("denied")},
		}},
		"unrelated": {Role: RoleUser, Content: Content{
			ToolResultBlock{ToolUseID: "other", Content: Text("other output")},
		}},
		"user": {Role: RoleUser, Content: Text("continue")},
		"hidden": {
			Role: RoleUser, Visibility: MessageHidden,
			Content: Text("other hidden context"),
		},
		"next round": {RoundID: otherRoundID, Role: RoleUser, Content: Content{
			ToolResultBlock{ToolUseID: "read", Content: Text("different round")},
		}},
	}
	for name, message := range messages {
		message.ID = uuid.New()
		if message.RoundID == uuid.Nil {
			message.RoundID = roundID
		}
		messages[name] = message
	}

	for _, tt := range []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "multiple updates and split results",
			input: []string{"user", "calls", "auto", "read", "yolo", "rest", "user"},
			want:  []string{"user", "calls", "read", "rest", "auto", "yolo", "user"},
		},
		{
			name:  "consecutive batches",
			input: []string{"calls", "auto", "read", "rest", "calls", "yolo", "read", "rest"},
			want:  []string{"calls", "read", "rest", "auto", "calls", "read", "rest", "yolo"},
		},
		{
			name:  "already ordered",
			input: []string{"auto", "calls", "read", "rest", "yolo"},
			want:  []string{"auto", "calls", "read", "rest", "yolo"},
		},
		{
			name:  "incomplete batch stays intact",
			input: []string{"calls", "auto", "read", "yolo"},
			want:  []string{"calls", "auto", "read", "yolo"},
		},
		{
			name:  "unrelated result cannot close a batch",
			input: []string{"calls", "auto", "read", "unrelated", "rest"},
			want:  []string{"calls", "auto", "read", "unrelated", "rest"},
		},
		{
			name:  "visible input is a boundary",
			input: []string{"calls", "auto", "user", "read", "rest"},
			want:  []string{"calls", "auto", "user", "read", "rest"},
		},
		{
			name:  "other hidden context is a boundary",
			input: []string{"calls", "auto", "hidden", "read", "rest"},
			want:  []string{"calls", "auto", "hidden", "read", "rest"},
		},
		{
			name:  "round boundary",
			input: []string{"calls", "auto", "next round", "rest"},
			want:  []string{"calls", "auto", "next round", "rest"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := make([]Message, 0, len(tt.input))
			want := make([]Message, 0, len(tt.want))
			for _, name := range tt.input {
				input = append(input, messages[name])
			}
			for _, name := range tt.want {
				want = append(want, messages[name])
			}
			session := &Session{context: append([]Message{}, input...)}
			for range 2 {
				if got := session.ContextMessages(); !reflect.DeepEqual(got, want) {
					t.Fatalf("context messages do not match order %v", tt.want)
				}
			}
			if !reflect.DeepEqual(session.context, input) {
				t.Fatal("context projection changed the source messages")
			}
		})
	}
}

func TestContextMessagesRepairsReplayedFailedRound(t *testing.T) {
	t.Parallel()

	session := StartSession(shared.NewModelRef("deepseek", "test-model"), 128_000, shared.ReasoningOff, nil)
	roundID, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	call, err := session.AppendAssistantMessage(roundID, Content{
		ToolUseBlock{ID: "call", Name: "read_file", Input: []byte(`{}`)},
	}, *session.CurrentModel, nil)
	if err != nil {
		t.Fatal(err)
	}
	session.SetPermissionMode(PermissionYolo, roundID)
	metadata, err := session.AppendHiddenMessage(
		roundID,
		RoleDeveloper,
		Text("<metadata><permission-mode>yolo</permission-mode></metadata>"),
		shared.Metadata{"kind": "metadata", "scope": "round"},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.AppendUserMessage(roundID, Content{
		ToolResultBlock{ToolUseID: "call", Content: Text("saved result")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.CompleteRound(roundID, RoundFailed, TokenUsage{}, new("No tool output found")); err != nil {
		t.Fatal(err)
	}

	replayed := ReplaySession(roundTripEvents(t, session.PendingEvents()))
	raw := []Message{call, metadata, result}
	if !reflect.DeepEqual(replayed.Rounds[0].Messages, raw) {
		t.Fatal("replay changed the persisted message order")
	}
	if replayed.CurrentPermissionMode() != PermissionYolo {
		t.Fatal("replay lost the permission mode")
	}
	if got := replayed.ContextMessages(); !reflect.DeepEqual(got, []Message{call, result, metadata}) {
		t.Fatal("failed round was not repaired for model context")
	}
	if len(replayed.PendingEvents()) != 0 {
		t.Fatal("context projection created persistence events")
	}
}
