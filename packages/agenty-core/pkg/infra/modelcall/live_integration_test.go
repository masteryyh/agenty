//go:build integration

package modelcall

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

func TestLiveProviders(t *testing.T) {
	tests := []struct {
		name         string
		apiType      APIType
		keyEnv       string
		baseURLEnv   string
		modelEnv     string
		defaultModel string
	}{
		{
			name: "OpenAI Responses", apiType: APIOpenAI,
			keyEnv: "OPENAI_API_KEY", baseURLEnv: "OPENAI_BASE_URL",
			modelEnv: "OPENAI_RESPONSES_MODEL", defaultModel: "gpt-5-mini",
		},
		{
			name: "OpenAI Chat Completions", apiType: APIOpenAICompletions,
			keyEnv: "OPENAI_API_KEY", baseURLEnv: "OPENAI_BASE_URL",
			modelEnv: "OPENAI_CHAT_MODEL", defaultModel: "gpt-4.1-mini",
		},
		{
			name: "Anthropic Messages", apiType: APIAnthropic,
			keyEnv: "ANTHROPIC_API_KEY", baseURLEnv: "ANTHROPIC_BASE_URL",
			modelEnv: "ANTHROPIC_MODEL", defaultModel: "claude-haiku-4-5",
		},
		{
			name: "Google GenAI", apiType: APIGemini,
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
				modelCode = configured
			}
			model := Config{
				APIType:   tt.apiType,
				APIKey:    apiKey,
				BaseURL:   os.Getenv(tt.baseURLEnv),
				ModelCode: modelCode,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			request := Request{
				Messages: []Message{{
					Role:    conversation.RoleUser,
					Content: conversation.Text("Reply with exactly OK."),
				}},
				MaxOutputTokens: 64,
			}

			t.Run("invoke", func(t *testing.T) {
				response, err := Call(ctx, model, request)
				if err != nil {
					t.Fatalf("invoke: %v", err)
				}
				if len(response.Content) == 0 {
					t.Fatal("invoke returned no content")
				}
			})

			t.Run("stream", func(t *testing.T) {
				completed := false
				response, err := Call(ctx, model, request, WithStreamHandler(func(event StreamEvent) error {
					if event.Type == StreamEventCompleted {
						completed = true
					}
					return nil
				}))
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
