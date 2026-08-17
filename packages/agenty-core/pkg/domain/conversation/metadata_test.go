package conversation

import (
	"strings"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestMetadataUpdateXML(t *testing.T) {
	t.Parallel()

	cwd := `/workspace/<shared>&` // XML escaping is part of the persisted contract.
	update := (SessionMetadata{
		Cwd:             cwd,
		Model:           "model",
		Provider:        "provider",
		Timezone:        "Asia/Shanghai",
		ReasoningEffort: string(shared.ReasoningHigh),
	}).Diff(nil)

	got, err := update.XML()
	if err != nil {
		t.Fatalf("MetadataUpdate.XML: %v", err)
	}
	for _, want := range []string{
		"<metadata>",
		"<cwd>/workspace/&lt;shared&gt;&amp;</cwd>",
		"<reasoning-effort>high</reasoning-effort>",
		"</metadata>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("metadata XML = %q, missing %q", got, want)
		}
	}
}

func TestReplaySessionCachesMetadataAndVisibleCopyFiltersIt(t *testing.T) {
	t.Parallel()

	session := StartSession(
		"coder",
		shared.NewModelRef("provider", "model"),
		128_000,
		shared.ReasoningHigh,
		stringPointerForTest("/workspace/one"),
	)
	roundID, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	first := SessionMetadata{
		Cwd:             "/workspace/one",
		Model:           "model",
		Provider:        "provider",
		Timezone:        "Asia/Shanghai",
		ReasoningEffort: "high",
	}
	firstXML, err := first.Diff(nil).XML()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendHiddenUserMessage(roundID, Text(firstXML)); err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendUserMessage(roundID, Text("hello")); err != nil {
		t.Fatal(err)
	}

	replayed := ReplaySession(session.PendingEvents())
	metadata := replayed.LastMetadata()
	if metadata == nil || *metadata != first {
		t.Fatalf("cached metadata = %+v, want %+v", metadata, first)
	}
	visible := replayed.VisibleCopy()
	if len(visible.Rounds) != 1 || len(visible.Rounds[0].Messages) != 1 {
		t.Fatalf("visible messages = %+v, want one user message", visible.Rounds[0].Messages)
	}
	if got := visible.Rounds[0].Messages[0].Content[0].(TextBlock).Text; got != "hello" {
		t.Errorf("visible message = %q, want hello", got)
	}
}

func TestReplaySessionMergesIncrementalMetadata(t *testing.T) {
	t.Parallel()

	session := StartSession(
		"coder",
		shared.NewModelRef("provider", "model"),
		128_000,
		shared.ReasoningHigh,
		nil,
	)
	roundID, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	initial := SessionMetadata{
		Cwd:             "/workspace/one",
		Model:           "model",
		Provider:        "provider",
		Timezone:        "Asia/Shanghai",
		ReasoningEffort: "high",
	}
	initialXML, err := initial.Diff(nil).XML()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendHiddenUserMessage(roundID, Text(initialXML)); err != nil {
		t.Fatal(err)
	}
	updatedCwd := "/workspace/two"
	updateXML, err := (MetadataUpdate{Cwd: &updatedCwd}).XML()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendHiddenUserMessage(roundID, Text(updateXML)); err != nil {
		t.Fatal(err)
	}

	replayed := ReplaySession(session.PendingEvents())
	want := initial
	want.Cwd = updatedCwd
	if got := replayed.LastMetadata(); got == nil || *got != want {
		t.Fatalf("cached incremental metadata = %+v, want %+v", got, want)
	}
}

func stringPointerForTest(value string) *string {
	return &value
}
