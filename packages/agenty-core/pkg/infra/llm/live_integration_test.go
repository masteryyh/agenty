//go:build integration

package llm

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestLiveProviders(t *testing.T) {
	tests := []struct {
		name         string
		apiType      catalog.APIType
		providerCode shared.Code
		keyEnv       string
		baseURLEnv   string
		modelEnv     string
		defaultModel shared.ModelCode
	}{
		{
			name: "OpenAI Responses", apiType: catalog.APIOpenAI, providerCode: "openai",
			keyEnv: "OPENAI_API_KEY", baseURLEnv: "OPENAI_BASE_URL",
			modelEnv: "OPENAI_RESPONSES_MODEL", defaultModel: "gpt-5-mini",
		},
		{
			name: "OpenAI Chat Completions", apiType: catalog.APIOpenAICompletions, providerCode: "openai-completions",
			keyEnv: "OPENAI_API_KEY", baseURLEnv: "OPENAI_BASE_URL",
			modelEnv: "OPENAI_CHAT_MODEL", defaultModel: "gpt-4.1-mini",
		},
		{
			name: "Anthropic Messages", apiType: catalog.APIAnthropic, providerCode: "anthropic",
			keyEnv: "ANTHROPIC_API_KEY", baseURLEnv: "ANTHROPIC_BASE_URL",
			modelEnv: "ANTHROPIC_MODEL", defaultModel: "claude-haiku-4-5",
		},
		{
			name: "Google GenAI", apiType: catalog.APIGemini, providerCode: "google",
			keyEnv: "GEMINI_API_KEY", baseURLEnv: "GEMINI_BASE_URL",
			modelEnv: "GEMINI_MODEL", defaultModel: "gemini-2.5-flash",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiKey := os.Getenv(tt.keyEnv)
			if apiKey == "" {
				t.Skipf("%s is not set; skipping live %s integration test", tt.keyEnv, tt.name)
			}

			modelCode := tt.defaultModel
			if configured := os.Getenv(tt.modelEnv); configured != "" {
				modelCode = shared.ModelCode(configured)
			}
			provider := catalog.Provider{
				Code: tt.providerCode, Type: tt.apiType,
				APIKey: apiKey, BaseURL: os.Getenv(tt.baseURLEnv),
			}
			model := catalog.Model{Code: modelCode}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			caller, err := NewCaller(ctx, provider, model)
			if err != nil {
				t.Fatalf("create caller: %v", err)
			}
			request := modelRequest{
				Messages: []conversation.Message{{
					Role:    conversation.RoleUser,
					Content: conversation.Text("Reply with exactly OK."),
				}},
				MaxOutputTokens: 64,
			}

			t.Run("invoke", func(t *testing.T) {
				response, err := caller.Invoke(ctx, request)
				if err != nil {
					t.Fatalf("invoke: %v", err)
				}
				if len(response.Content) == 0 {
					t.Fatal("invoke returned no content")
				}
			})

			t.Run("stream", func(t *testing.T) {
				completed := false
				response, err := caller.Stream(ctx, request, func(event modelStreamEvent) error {
					if event.Type == modelStreamEventCompleted {
						completed = true
					}
					return nil
				})
				if err != nil {
					t.Fatalf("stream: %v", err)
				}
				if !completed {
					t.Fatal("stream did not emit a completed event")
				}
				if len(response.Content) == 0 {
					t.Fatal("stream returned no content")
				}
			})
		})
	}
}
