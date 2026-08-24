package modelcatalog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestListerOpenAICompatibleDefaultsAndMissingReasoning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %q, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer openai-secret" {
			t.Errorf("authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-test"}]}`))
	}))
	defer server.Close()

	models, err := NewLister(server.Client()).List(t.Context(), catalog.Provider{
		Code:    "openai",
		Type:    catalog.APIOpenAI,
		BaseURL: server.URL + "/v1",
		APIKey:  "openai-secret",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []catalog.AvailableModel{{
		Code:             "gpt-test",
		Name:             "gpt-test",
		ContextWindow:    catalog.DefaultAvailableModelContextWindow,
		MaxOutputTokens:  catalog.DefaultAvailableModelMaxOutputTokens,
		ReasoningEfforts: []shared.ReasoningEffort{},
	}}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("models = %#v, want %#v", models, want)
	}
}

func TestListerDeepSeekUsesOpenAICompatibleShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path = %q, want /models", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"deepseek-chat","owned_by":"deepseek"}]}`))
	}))
	defer server.Close()

	models, err := NewLister(server.Client()).List(t.Context(), catalog.Provider{
		Code:    "deepseek",
		Type:    catalog.APIOpenAICompletions,
		BaseURL: server.URL,
		APIKey:  "deepseek-secret",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(models) != 1 || models[0].Code != "deepseek-chat" || models[0].ReasoningEfforts == nil || len(models[0].ReasoningEfforts) != 0 {
		t.Fatalf("models = %#v", models)
	}
}

func TestListerAnthropicPaginationAndCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "anthropic-secret" {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != defaultAnthropicVersion {
			t.Errorf("anthropic-version = %q", got)
		}
		if r.URL.Query().Get("after_id") == "" {
			_, _ = w.Write([]byte(`{
                "data":[{
                    "id":"claude-opus",
                    "display_name":"",
                    "max_input_tokens":200000,
                    "max_tokens":64000,
                    "capabilities":{"image_input":{"supported":true},"effort":{"low":{"supported":true},"max":{"supported":true}}}
                }],
                "has_more":true,
                "last_id":"claude-opus"
            }`))
			return
		}
		if got := r.URL.Query().Get("after_id"); got != "claude-opus" {
			t.Errorf("after_id = %q", got)
		}
		_, _ = w.Write([]byte(`{
            "data":[{"id":"claude-haiku","max_input_tokens":0,"max_tokens":0}],
            "has_more":false,
            "last_id":"claude-haiku"
        }`))
	}))
	defer server.Close()

	models, err := NewLister(server.Client()).List(t.Context(), catalog.Provider{
		Code:    "anthropic",
		Type:    catalog.APIAnthropic,
		BaseURL: server.URL,
		APIKey:  "anthropic-secret",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("models = %#v", models)
	}
	if models[0].Name != "claude-opus" || models[0].ContextWindow != 200000 || models[0].MaxOutputTokens != 64000 || !models[0].MultiModal {
		t.Errorf("first model = %#v", models[0])
	}
	if !reflect.DeepEqual(models[0].ReasoningEfforts, []shared.ReasoningEffort{shared.ReasoningLow, shared.ReasoningMax}) {
		t.Errorf("first reasoning efforts = %#v", models[0].ReasoningEfforts)
	}
	if models[1].Name != "claude-haiku" || models[1].ContextWindow != catalog.DefaultAvailableModelContextWindow || models[1].MaxOutputTokens != catalog.DefaultAvailableModelMaxOutputTokens || len(models[1].ReasoningEfforts) != 0 {
		t.Errorf("second model = %#v", models[1])
	}
}

func TestListerOpenRouterFieldsAndReasoning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := map[string]any{
			"data": []map[string]any{
				{
					"id":             "openai/gpt-test",
					"name":           "",
					"context_length": 128000,
					"architecture":   map[string]any{"input_modalities": []string{"text", "image"}},
					"top_provider":   map[string]any{"max_completion_tokens": 32768},
					"reasoning":      map[string]any{"supported_efforts": []string{"high", "minimal", "low"}},
				},
				{"id": "plain-model"},
			},
		}
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	models, err := NewLister(server.Client()).List(t.Context(), catalog.Provider{
		Code:    "openrouter",
		Type:    catalog.APIOpenAICompletions,
		BaseURL: server.URL + "/api/v1",
		APIKey:  "openrouter-secret",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("models = %#v", models)
	}
	if models[0].Name != "openai/gpt-test" || models[0].ContextWindow != 128000 || models[0].MaxOutputTokens != 32768 || !models[0].MultiModal {
		t.Errorf("first model = %#v", models[0])
	}
	if !reflect.DeepEqual(models[0].ReasoningEfforts, []shared.ReasoningEffort{shared.ReasoningLow, shared.ReasoningHigh}) {
		t.Errorf("first reasoning efforts = %#v", models[0].ReasoningEfforts)
	}
	if models[1].ContextWindow != catalog.DefaultAvailableModelContextWindow || models[1].MaxOutputTokens != catalog.DefaultAvailableModelMaxOutputTokens || len(models[1].ReasoningEfforts) != 0 {
		t.Errorf("second model = %#v", models[1])
	}
}

func TestListerOpenRouterNullReasoningMeansAllStandardEfforts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"reasoning-model","reasoning":{"supported_efforts":null}}]}`))
	}))
	defer server.Close()

	models, err := NewLister(server.Client()).List(t.Context(), catalog.Provider{
		Code:    "openrouter",
		Type:    catalog.APIOpenAI,
		BaseURL: server.URL,
		APIKey:  "secret",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !reflect.DeepEqual(models[0].ReasoningEfforts, shared.StandardReasoningEfforts()) {
		t.Fatalf("reasoning efforts = %#v", models[0].ReasoningEfforts)
	}
}

func TestListerGeminiPaginationAndThinking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("key"); got != "gemini-secret" {
			t.Errorf("key = %q", got)
		}
		if r.URL.Query().Get("pageToken") == "" {
			_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-3-flash","displayName":"Gemini 3 Flash","inputTokenLimit":128000,"outputTokenLimit":8192,"thinking":true}],"nextPageToken":"next"}`))
			return
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-3-pro"}],"nextPageToken":""}`))
	}))
	defer server.Close()

	models, err := NewLister(server.Client()).List(t.Context(), catalog.Provider{
		Code:    "google",
		Type:    catalog.APIGemini,
		BaseURL: server.URL,
		APIKey:  "gemini-secret",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(models) != 2 || models[0].Code != "gemini-3-flash" || models[0].Name != "Gemini 3 Flash" || len(models[0].ReasoningEfforts) != len(shared.StandardReasoningEfforts()) {
		t.Fatalf("models = %#v", models)
	}
	if models[1].Name != "gemini-3-pro" || len(models[1].ReasoningEfforts) != 0 {
		t.Fatalf("second model = %#v", models[1])
	}
}

func TestEndpointURLRejectsAbsoluteModelsURL(t *testing.T) {
	_, err := endpointURL(catalog.Provider{
		Type:      catalog.APIOpenAI,
		BaseURL:   "https://example.com/v1",
		ModelsURL: "https://other.example/models",
	})
	if err == nil {
		t.Fatal("endpointURL error = nil, want error")
	}
}
