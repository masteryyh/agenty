package agentloop

import (
	"strings"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestCompactionThresholdReservesOutputBudget(t *testing.T) {
	t.Parallel()

	if DefaultMaxOutputTokens != 8_192 {
		t.Fatalf("default max output tokens = %d", DefaultMaxOutputTokens)
	}
	if CompactionThreshold(100_000, 8_192) != 90_000 {
		t.Fatalf("threshold = %d", CompactionThreshold(100_000, 8_192))
	}
	if CompactionThreshold(200_000, 64_000) != 136_000 {
		t.Fatalf("large-output threshold = %d", CompactionThreshold(200_000, 64_000))
	}
	if ShouldCompact(89_999, 100_000, 8_192) {
		t.Error("context below 90 percent compacted")
	}
	if !ShouldCompact(90_000, 100_000, 8_192) {
		t.Error("threshold boundary did not compact")
	}
	if !ShouldCompact(136_000, 200_000, 64_000) {
		t.Error("maximum-output boundary did not compact")
	}
}

func TestFitCompactedRequestDropsAssistantContextBeforeUserContext(t *testing.T) {
	t.Parallel()

	request := Request{
		Messages: []conversation.Message{
			compactionTestMessage(conversation.RoleUser, "retained_user", "user"),
			compactionTestMessage(conversation.RoleUser, "summary", "summary"),
			compactionTestMessage(conversation.RoleUser, "metadata", "metadata"),
			compactionTestMessage(conversation.RoleAssistant, "retained_assistant", strings.Repeat("assistant ", 40)),
		},
	}

	fitted := fitCompactedRequest(request, 100)
	if len(fitted.Messages) != 3 {
		t.Fatalf("fitted messages = %+v, want retained user, summary, metadata", fitted.Messages)
	}
	if compactionKind(fitted.Messages[0]) != "retained_user" {
		t.Fatalf("retained context = %+v", fitted.Messages)
	}
}

func compactionTestMessage(role conversation.Role, kind string, text string) conversation.Message {
	return conversation.Message{
		Role:     role,
		Metadata: shared.Metadata{"compactionKind": kind},
		Content:  conversation.Text(text),
	}
}
