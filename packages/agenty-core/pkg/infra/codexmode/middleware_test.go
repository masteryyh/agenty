package codexmode

import (
	"strings"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	inframiddleware "github.com/masteryyh/agenty-core/pkg/infra/middleware"
)

func TestMiddlewareAddsApplyPatchGuidanceOnlyInCodexMode(t *testing.T) {
	provider, err := catalog.NewProvider("openai", "OpenAI", catalog.APIOpenAI)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		dialect conversation.ToolDialect
		want    string
		notWant string
	}{
		{
			name:    "codex",
			dialect: conversation.ToolDialectCodex,
			want:    "complete V4A patch",
			notWant: "This session uses Codex Mode",
		},
		{
			name:    "default",
			dialect: conversation.ToolDialectDefault,
			want:    "str_replace_based_edit_tool",
			notWant: "complete V4A patch",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := conversation.StartSessionWithModes(
				shared.NewModelRef("openai", "gpt-test"),
				128_000,
				shared.ReasoningOff,
				nil,
				conversation.PermissionAsk,
				test.dialect,
			)
			prompt := "<basic>base</basic>"
			state := &inframiddleware.SessionStartContext{
				Session:      session,
				Provider:     provider,
				SystemPrompt: &prompt,
			}
			if err := beforeSessionStart(t.Context(), state); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(prompt, test.want) {
				t.Fatalf("system prompt does not contain %q: %s", test.want, prompt)
			}
			if strings.Contains(prompt, test.notWant) {
				t.Fatalf("system prompt contains %q: %s", test.notWant, prompt)
			}
		})
	}
}
