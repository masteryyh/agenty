package inspection

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-inspector/internal/testfixture"
)

func TestBuildMatchesCoreAndPreservesSources(t *testing.T) {
	t.Parallel()
	events := testfixture.Events()
	raw := testfixture.Encode(events)
	snapshot, err := Build(t.Context(), SessionEntry{ID: "fixture"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	want := conversation.ReplaySession(events)
	if !reflect.DeepEqual(snapshot.session, want) {
		t.Fatal("inspector replay differs from core")
	}
	if snapshot.Detail.HiddenCount != 1 || len(snapshot.Detail.Rounds) != 2 {
		t.Fatalf("missing hidden messages or rounds: %+v", snapshot.Detail)
	}
	if len(snapshot.Detail.Tools) != 3 {
		t.Fatalf("tool relations = %d", len(snapshot.Detail.Tools))
	}
	for _, tool := range snapshot.Detail.Tools {
		if tool.Status != "matched" || len(tool.Calls) != 1 || len(tool.Results) != 1 {
			t.Fatalf("incorrect relation: %+v", tool)
		}
	}
	for i, record := range snapshot.Records {
		detail, exists := snapshot.Record(record.ID)
		if !exists || detail.Raw != string(bytes.SplitAfter(raw, []byte{'\n'})[i]) {
			t.Fatalf("raw line %d is not exact", i+1)
		}
		view, exists, err := snapshot.Context(t.Context(), record.ID)
		if err != nil || !exists {
			t.Fatalf("context: %v, exists %v", err, exists)
		}
		coreContext := conversation.ReplaySession(events[:i+1]).ContextMessages()
		actual := []conversation.Message{}
		for _, node := range view.After {
			actual = append(actual, node.Message)
			if len(node.Sources) == 0 {
				t.Errorf("missing provenance for %s at line %d", node.Kind, i+1)
			}
		}
		if !reflect.DeepEqual(actual, coreContext) {
			t.Fatalf("context differs at line %d", i+1)
		}
	}
}

func TestProjectUsesMetadataCwdWhenLegacyStartHasNoCwd(t *testing.T) {
	t.Parallel()

	model := shared.NewModelRef("provider", "model")
	session := conversation.StartSession(model, 128_000, shared.ReasoningHigh, nil)
	roundID, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	_, err = session.AppendHiddenUserMessage(roundID, conversation.Text("<metadata><cwd>/workspace/legacy</cwd></metadata>"))
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := Build(t.Context(), SessionEntry{ID: "legacy"}, testfixture.Encode(session.PendingEvents()))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Detail.Cwd == nil || *snapshot.Detail.Cwd != "/workspace/legacy" {
		t.Fatalf("cwd = %v, want metadata cwd", snapshot.Detail.Cwd)
	}
}

func TestProjectDoesNotResurrectExplicitlyClearedCwd(t *testing.T) {
	t.Parallel()

	model := shared.NewModelRef("provider", "model")
	session := conversation.StartSession(model, 128_000, shared.ReasoningHigh, nil)
	roundID, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	_, err = session.AppendHiddenUserMessage(roundID, conversation.Text("<metadata><cwd>/workspace/legacy</cwd></metadata>"))
	if err != nil {
		t.Fatal(err)
	}
	session.SetCwd(nil)

	snapshot, err := Build(t.Context(), SessionEntry{ID: "cleared"}, testfixture.Encode(session.PendingEvents()))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Detail.Cwd != nil {
		t.Fatalf("cwd = %q, want explicit clear", *snapshot.Detail.Cwd)
	}
}

func TestBuildStopsReplayButKeepsBadAndLaterLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		bad  string
		code string
	}{
		{name: "malformed middle", bad: "{bad}\n", code: "decode_error"},
		{name: "unknown event", bad: "{\"type\":\"future_event\",\"seq\":2,\"payload\":{}}\n", code: "decode_error"},
		{name: "unknown block", bad: "{\"type\":\"message_appended\",\"seq\":2,\"payload\":{\"message\":{\"content\":[{\"type\":\"future_block\"}]}}}\n", code: "decode_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := testfixture.Events()
			first := testfixture.Encode(events[:1])
			raw := append(append(first, []byte(tt.bad)...), testfixture.Encode(events[2:3])...)
			snapshot, err := Build(t.Context(), SessionEntry{}, raw)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Detail.Complete || snapshot.Detail.ReplayedThrough != 1 || len(snapshot.Records) != 3 {
				t.Fatalf("invalid partial replay: %+v", snapshot.Detail)
			}
			if got, _ := snapshot.Record(snapshot.Records[1].ID); got.Raw != tt.bad {
				t.Fatal("bad raw line was lost")
			}
			if snapshot.Records[2].Applied {
				t.Fatal("later event was applied after a decoding gap")
			}
		})
	}
}

func TestBuildTailSequenceAndLargeContent(t *testing.T) {
	t.Parallel()
	events := testfixture.Events()
	tests := []struct {
		name     string
		raw      []byte
		complete bool
		code     string
	}{
		{name: "valid no newline", raw: bytes.TrimSuffix(testfixture.Encode(events), []byte{'\n'}), complete: true, code: "unterminated_record"},
		{name: "partial tail", raw: append(testfixture.Encode(events), []byte("{\"type\":")...), code: "trailing_fragment"},
		{name: "duplicate seq", raw: append(testfixture.Encode(events[:1]), testfixture.Encode(events[:1])...), complete: true, code: "event_sequence"},
		{name: "empty line", raw: append([]byte{'\n'}, testfixture.Encode(events)...), complete: true, code: "empty_line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := Build(t.Context(), SessionEntry{}, tt.raw)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Detail.Complete != tt.complete {
				t.Fatalf("complete = %v", snapshot.Detail.Complete)
			}
			found := false
			for _, issue := range snapshot.Detail.Diagnostics {
				found = found || issue.Code == tt.code
			}
			if !found {
				t.Fatalf("missing diagnostic %s", tt.code)
			}
		})
	}
	message := events[3].(conversation.MessageAppended)
	message.Message.Content = conversation.Text(strings.Repeat("x", 2<<20))
	large := testfixture.Encode([]shared.Event{events[0], events[2], message})
	snapshot, err := Build(t.Context(), SessionEntry{}, large)
	if err != nil || !snapshot.Detail.Complete {
		t.Fatalf("large content failed: %v", err)
	}
}

func TestBuildDiagnosesToolAmbiguityAndLifecycle(t *testing.T) {
	t.Parallel()
	events := testfixture.Events()
	assistant := events[4].(conversation.MessageAppended)
	// Locate by event kind rather than relying on fixture block positions.
	for _, event := range events {
		if e, ok := event.(conversation.MessageAppended); ok && e.Message.Role == conversation.RoleAssistant {
			assistant = e
			break
		}
	}
	events = append(events, assistant, assistant)
	snapshot, err := Build(t.Context(), SessionEntry{}, testfixture.Encode(events))
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, diagnostic := range snapshot.Detail.Diagnostics {
		codes[diagnostic.Code] = true
	}
	for _, code := range []string{"ambiguous", "duplicate_message", "message_after_end", "no_terminal_event"} {
		if !codes[code] {
			t.Errorf("missing %s", code)
		}
	}
	for _, diagnostic := range snapshot.Detail.Diagnostics {
		if diagnostic.Code == "ambiguous" && (!strings.Contains(diagnostic.Message, "calls") ||
			!strings.Contains(diagnostic.Message, "results") ||
			!strings.Contains(diagnostic.Message, "unique pairing")) {
			t.Errorf("ambiguous diagnostic is not actionable: %q", diagnostic.Message)
		}
	}
}

func TestBuildTreatsWrappedShellOutputAsOneResult(t *testing.T) {
	t.Parallel()
	events := testfixture.Events()
	wrapped := false
	for index, event := range events {
		messageEvent, ok := event.(conversation.MessageAppended)
		if !ok {
			continue
		}
		for blockIndex, block := range messageEvent.Message.Content {
			shellOutput, ok := block.(conversation.ShellCallOutputBlock)
			if !ok {
				continue
			}
			if len(shellOutput.Output) > 0 {
				shellOutput.Output = append(shellOutput.Output, shellOutput.Output[0], shellOutput.Output[0])
			}
			messageEvent.Message.Content[blockIndex] = conversation.ToolResultBlock{
				ToolUseID: shellOutput.CallID,
				Content:   conversation.Content{shellOutput},
			}
			events[index] = messageEvent
			wrapped = true
		}
	}
	if !wrapped {
		t.Fatal("fixture does not contain a shell output")
	}

	snapshot, err := Build(t.Context(), SessionEntry{}, testfixture.Encode(events))
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range snapshot.Detail.Diagnostics {
		if diagnostic.Code == "ambiguous" {
			t.Fatalf("wrapped shell output was marked ambiguous: %q", diagnostic.Message)
		}
	}
	found := false
	for _, relation := range snapshot.Detail.Tools {
		if relation.ID != "shell-1" {
			continue
		}
		found = true
		if relation.Status != "matched" || len(relation.Calls) != 1 || len(relation.Results) != 1 {
			t.Fatalf("shell relation = %+v, want one matched call/result", relation)
		}
	}
	if !found {
		t.Fatal("shell relation was not recorded")
	}
}

func TestRawPreservesInvalidUTF8AndSearchesFullText(t *testing.T) {
	t.Parallel()
	raw := append(testfixture.Encode(testfixture.Events()), []byte{'{', 0xff, '}', '\n'}...)
	snapshot, err := Build(t.Context(), SessionEntry{}, raw)
	if err != nil {
		t.Fatal(err)
	}
	last := snapshot.Records[len(snapshot.Records)-1]
	detail, ok := snapshot.Record(last.ID)
	if !ok || detail.RawBase64 != "e/99Cg==" {
		t.Fatalf("original invalid bytes lost: %q", detail.RawBase64)
	}
	records, err := snapshot.Search(t.Context(), "config.go", "message_appended", "")
	if err != nil || len(records) != 1 {
		t.Fatalf("full raw search failed: %d matches, %v", len(records), err)
	}
}
