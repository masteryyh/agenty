package catalogdata

import "testing"

func TestLoadProviders(t *testing.T) {
	providers, err := LoadProviders()
	if err != nil {
		t.Fatalf("LoadProviders: %v", err)
	}
	if len(providers) != 5 {
		t.Fatalf("providers = %d, want 5", len(providers))
	}

	byCode := make(map[string]struct {
		name     string
		typeName string
	})
	for _, provider := range providers {
		if !provider.Builtin {
			t.Errorf("provider %s is not built in", provider.Code)
		}
		if provider.Code != "openrouter" && !provider.Official {
			t.Errorf("provider %s is not official", provider.Code)
		}
		byCode[provider.Code.String()] = struct {
			name     string
			typeName string
		}{provider.Name, string(provider.Type)}
	}
	if got := byCode["openai_legacy"]; got.name != "OpenAI (Legacy API)" || got.typeName != "openai_completions" {
		t.Errorf("legacy OpenAI provider = %+v", got)
	}
	openRouter := providers[2]
	if openRouter.Code != "openrouter" || openRouter.Name != "OpenRouter" || openRouter.Type != "openai" {
		t.Errorf("OpenRouter provider = %+v", openRouter)
	}
	if openRouter.Official || openRouter.BaseURL != "https://openrouter.ai/api/v1" || openRouter.ModelsURL != "models" {
		t.Errorf("OpenRouter discovery config = %+v", openRouter)
	}
	if openRouter.Models == nil || len(openRouter.Models) != 0 {
		t.Errorf("OpenRouter embedded models = %#v, want empty", openRouter.Models)
	}
	openAI := providers[0]
	if openAI.Code != "openai" || !openAI.FreeFormTool {
		t.Errorf("OpenAI freeFormTool = %v, want true", openAI.FreeFormTool)
	}
	for _, provider := range providers {
		for _, model := range provider.Models {
			if model.ContextWindow <= 0 || model.MaxOutputTokens <= 0 {
				t.Errorf("model %s/%s has invalid token limits", provider.Code, model.Code)
			}
			if model.ReasoningEfforts == nil {
				t.Errorf("model %s/%s has nil reasoning efforts", provider.Code, model.Code)
			}
		}
	}
}
