//go:build e2e

package e2e_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

type liveProviderCase struct {
	name         string
	apiType      string
	prefix       string
	keyEnv       string
	baseURLEnv   string
	modelEnv     string
	defaultModel string
}

func TestLiveProviderConversationsThroughIPC(t *testing.T) {
	tests := []liveProviderCase{
		{
			name:         "OpenAI Responses",
			apiType:      "openai",
			prefix:       "live-openai-responses",
			keyEnv:       "OPENAI_API_KEY",
			baseURLEnv:   "OPENAI_BASE_URL",
			modelEnv:     "OPENAI_RESPONSES_MODEL",
			defaultModel: "gpt-5-mini",
		},
		{
			name:         "OpenAI Chat Completions",
			apiType:      "openai_completions",
			prefix:       "live-openai-chat",
			keyEnv:       "OPENAI_API_KEY",
			baseURLEnv:   "OPENAI_BASE_URL",
			modelEnv:     "OPENAI_CHAT_MODEL",
			defaultModel: "gpt-4.1-mini",
		},
		{
			name:         "Anthropic Messages",
			apiType:      "anthropic",
			prefix:       "live-anthropic",
			keyEnv:       "ANTHROPIC_API_KEY",
			baseURLEnv:   "ANTHROPIC_BASE_URL",
			modelEnv:     "ANTHROPIC_MODEL",
			defaultModel: "claude-haiku-4-5",
		},
		{
			name:         "Google GenAI",
			apiType:      "gemini",
			prefix:       "live-gemini",
			keyEnv:       "GEMINI_API_KEY",
			baseURLEnv:   "GEMINI_BASE_URL",
			modelEnv:     "GEMINI_MODEL",
			defaultModel: "gemini-2.5-flash",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runLiveProviderConversation(t, tt)
		})
	}
}

func runLiveProviderConversation(t *testing.T, tt liveProviderCase) {
	t.Helper()

	apiKey := strings.TrimSpace(os.Getenv(tt.keyEnv))
	if apiKey == "" {
		t.Skipf("%s is not set; skipping live %s E2E conversation", tt.keyEnv, tt.name)
	}
	modelSlug := strings.TrimSpace(os.Getenv(tt.modelEnv))
	if modelSlug == "" {
		modelSlug = tt.defaultModel
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	client := newAgentyClient(startCore(t))
	agentSlug := tt.prefix + "-agent"
	providerSlug := tt.prefix + "-provider"

	_, err := client.CreateAgent(ctx, AgentCreateInput{
		Slug: agentSlug,
		Name: "Live Provider E2E Agent",
		Soul: "Follow the user's requested output format exactly.",
	})
	requireNoError(t, err)
	_, err = client.CreateProvider(ctx, ProviderCreateInput{
		Slug:    providerSlug,
		Name:    tt.name,
		Type:    tt.apiType,
		BaseURL: strings.TrimSpace(os.Getenv(tt.baseURLEnv)),
		APIKey:  apiKey,
	})
	requireNoError(t, err)
	_, err = client.AddModel(ctx, ModelInput{
		ProviderSlug:    providerSlug,
		ModelSlug:       modelSlug,
		Name:            modelSlug,
		ContextWindow:   128_000,
		MaxOutputTokens: 64,
	})
	requireNoError(t, err)
	session, err := client.CreateSession(ctx, SessionCreateInput{
		AgentSlug:     agentSlug,
		ProviderSlug:  providerSlug,
		ModelSlug:     modelSlug,
		ContextWindow: 128_000,
	})
	requireNoError(t, err)

	started, err := client.StartSession(ctx, session.ID, []ContentInput{{
		Type: "text",
		Text: "Reply with exactly OK.",
	}})
	requireNoError(t, err)
	completed, err := client.WaitForRoundStatus(
		ctx,
		session.ID,
		started.RoundID,
		"completed",
	)
	requireNoError(t, err)
	if !roundHasAssistantText(completed, started.RoundID) {
		t.Fatalf("live %s conversation returned no assistant text", tt.name)
	}
}

func roundHasAssistantText(session Session, roundID string) bool {
	for _, round := range session.Rounds {
		if round.ID != roundID {
			continue
		}
		for _, message := range round.Messages {
			if message.Role != "assistant" {
				continue
			}
			for _, block := range message.Content {
				if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
					return true
				}
			}
		}
	}
	return false
}
