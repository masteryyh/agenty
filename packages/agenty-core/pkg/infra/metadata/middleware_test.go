package metadata

import (
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
