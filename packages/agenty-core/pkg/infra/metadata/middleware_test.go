package metadata

import (
	"strings"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	inframiddleware "github.com/masteryyh/agenty-core/pkg/infra/middleware"
)

func TestMiddlewareChoosesTheProviderMessageRole(t *testing.T) {
	tests := []struct {
		name    string
		apiType catalog.APIType
		want    conversation.Role
	}{
		{name: "developer capable", apiType: catalog.APIOpenAI, want: conversation.RoleDeveloper},
		{name: "user fallback", apiType: catalog.APIAnthropic, want: conversation.RoleUser},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, err := catalog.NewProvider("provider", "Provider", test.apiType)
			if err != nil {
				t.Fatal(err)
			}
			model := catalog.Model{Code: "model"}
			session := conversation.StartSession(
				shared.NewModelRef("provider", "model"),
				128_000,
				shared.ReasoningOff,
				nil,
			)
			var got conversation.Role
			state := &inframiddleware.SessionStartContext{
				Session:  session,
				Provider: provider,
				Model:    &model,
				AppendHiddenMessage: func(
					role conversation.Role,
					_ conversation.Content,
					_ map[string]any,
				) error {
					got = role
					return nil
				},
			}
			if err := NewMiddleware().BeforeSessionStart(t.Context(), state); err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("hidden metadata role = %q, want %q", got, test.want)
			}
		})
	}
}

func TestBuildMetadataIncludesPermissionMode(t *testing.T) {
	session := conversation.StartSessionWithPermission(
		shared.NewModelRef("provider", "model"),
		128_000,
		shared.ReasoningOff,
		nil,
		conversation.PermissionYolo,
	)
	metadata, err := buildMetadata(session, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := metadata.XML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(encoded, "<permission-mode>yolo</permission-mode>") {
		t.Fatalf("metadata XML = %q", encoded)
	}
}

func TestBeforeRoundEmitsPermissionModeAfterAnIdleSwitch(t *testing.T) {
	provider, err := catalog.NewProvider("provider", "Provider", catalog.APIOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	model := catalog.Model{Code: "model"}
	cwd := "/workspace"
	session := conversation.StartSession(
		shared.NewModelRef("provider", "model"),
		128_000,
		shared.ReasoningOff,
		&cwd,
	)
	roundID, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	initial := conversation.SessionMetadata{
		Cwd:             cwd,
		Model:           "model",
		Provider:        "provider",
		Timezone:        timezoneName(),
		ReasoningEffort: string(shared.ReasoningOff),
		PermissionMode:  conversation.PermissionAsk,
	}
	text, err := initial.XML()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendHiddenUserMessage(roundID, conversation.Text(text)); err != nil {
		t.Fatal(err)
	}
	if err := session.CompleteRound(roundID, conversation.RoundCompleted, conversation.TokenUsage{}, nil); err != nil {
		t.Fatal(err)
	}
	if !session.SetPermissionMode(conversation.PermissionYolo, roundID) {
		t.Fatal("permission mode change was not recorded")
	}

	var update string
	state := &inframiddleware.RoundContext{
		Session:  session,
		Provider: provider,
		Model:    &model,
		AppendHiddenMessage: func(_ conversation.Role, content conversation.Content, _ map[string]any) error {
			update = content[0].(conversation.TextBlock).Text
			return nil
		},
	}
	if err := NewMiddleware().BeforeRound(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(update, "<permission-mode>yolo</permission-mode>") {
		t.Fatalf("permission update = %q", update)
	}
}
