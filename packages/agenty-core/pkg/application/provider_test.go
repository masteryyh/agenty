package application_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestProviderCreateAndGet(t *testing.T) {
	_, providerSvc, _ := newServices(t)
	ctx := context.Background()

	p, err := providerSvc.Create(ctx, "anthropic", application.ProviderInput{
		Name:         "Anthropic",
		Type:         catalog.APIAnthropic,
		BaseURL:      "https://api.anthropic.com",
		APIKey:       "sk-ant-test",
		FreeFormTool: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Code.String() != "anthropic" {
		t.Errorf("code = %s", p.Code)
	}

	got, err := providerSvc.Get(ctx, "anthropic")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.APIKey != "sk-ant-test" {
		t.Errorf("apiKey = %s", got.APIKey)
	}
	if got.Type != catalog.APIAnthropic {
		t.Errorf("type = %s", got.Type)
	}
	if got.FreeFormTool {
		t.Error("freeFormTool = true for Anthropic provider, want ignored")
	}
}

func TestProviderCreateIgnoresFreeFormToolForNonOpenAI(t *testing.T) {
	for _, test := range []struct {
		code    string
		name    string
		apiType catalog.APIType
	}{
		{code: "anthropic", name: "Anthropic", apiType: catalog.APIAnthropic},
		{code: "google", name: "Google", apiType: catalog.APIGemini},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, providerSvc, _ := newServices(t)
			provider, err := providerSvc.Create(t.Context(), test.code, application.ProviderInput{
				Name:         test.name,
				Type:         test.apiType,
				FreeFormTool: true,
			})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if provider.FreeFormTool {
				t.Fatalf("freeFormTool = true for %s provider, want ignored", test.apiType)
			}
		})
	}
}

func TestProviderCreateInvalidType(t *testing.T) {
	_, providerSvc, _ := newServices(t)
	_, err := providerSvc.Create(context.Background(), "x", application.ProviderInput{
		Name: "X",
		Type: catalog.APIType("bogus"),
	})
	if code := appErrorCode(err); code != application.CodeValidation {
		t.Errorf("code = %v, want validation", code)
	}
}

func TestProviderCreateDuplicate(t *testing.T) {
	_, providerSvc, _ := newServices(t)
	if _, err := providerSvc.Create(t.Context(), "openai", application.ProviderInput{Name: "OpenAI", Type: catalog.APIOpenAI}); err != nil {
		t.Fatal(err)
	}
	_, err := providerSvc.Create(t.Context(), "openai", application.ProviderInput{Name: "Duplicate", Type: catalog.APIOpenAI})
	if code := appErrorCode(err); code != application.CodeAlreadyExists {
		t.Errorf("code = %v, want already_exists", code)
	}
}

func TestProviderList(t *testing.T) {
	_, providerSvc, _ := newServices(t)
	ctx := context.Background()

	for _, code := range []string{"anthropic", "openai"} {
		if _, err := providerSvc.Create(ctx, code, application.ProviderInput{Name: code, Type: catalog.APIOpenAI}); err != nil {
			t.Fatal(err)
		}
	}
	providers, err := providerSvc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(providers) != 2 {
		t.Errorf("List returned %d, want 2", len(providers))
	}
}

func TestProviderListModels(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.URL.Path != "/models" {
			t.Errorf("path = %q, want /models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-test"}]}`))
	}))
	defer server.Close()

	repo := newProviderRepositoryFake()
	provider, err := catalog.NewProvider("openai", "OpenAI", catalog.APIOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	provider.BaseURL = server.URL
	provider.APIKey = "test-key"
	repo.providers[provider.Code] = provider
	service := application.NewProviderService(repo)

	models, err := service.ListModels(t.Context(), "openai")
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 || models[0].Code != "gpt-test" {
		t.Fatalf("models = %#v", models)
	}
	if models[0].Name != "gpt-test" || models[0].ContextWindow != catalog.DefaultAvailableModelContextWindow || models[0].MaxOutputTokens != catalog.DefaultAvailableModelMaxOutputTokens {
		t.Fatalf("model defaults = %#v", models[0])
	}
	if models[0].ReasoningEfforts == nil || len(models[0].ReasoningEfforts) != 0 {
		t.Fatalf("reasoning efforts = %#v, want empty", models[0].ReasoningEfforts)
	}

	cached, err := service.ListModels(t.Context(), "openai")
	if err != nil {
		t.Fatalf("ListModels cached: %v", err)
	}
	if len(cached) != 1 || cached[0].Code != models[0].Code {
		t.Fatalf("cached models = %#v", cached)
	}
	if requestCount != 1 {
		t.Fatalf("model list requests = %d, want 1", requestCount)
	}
}

func TestProviderListModelsRequiresAPIKey(t *testing.T) {
	repo := newProviderRepositoryFake()
	provider, err := catalog.NewProvider("openai", "OpenAI", catalog.APIOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	repo.providers[provider.Code] = provider
	service := application.NewProviderService(repo)

	_, listErr := service.ListModels(t.Context(), "openai")
	if code := appErrorCode(listErr); code != application.CodeValidation {
		t.Fatalf("error code = %v, want validation", code)
	}
}

func TestProviderListAutomaticallyDiscoversOnlyRequestedProvider(t *testing.T) {
	var firstRequests atomic.Int32
	firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		firstRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-first"}]}`))
	}))
	defer firstServer.Close()

	var secondRequests atomic.Int32
	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-second"}]}`))
	}))
	defer secondServer.Close()

	repo := newProviderRepositoryFake()
	for _, item := range []struct {
		code    string
		baseURL string
	}{
		{code: "first", baseURL: firstServer.URL},
		{code: "second", baseURL: secondServer.URL},
	} {
		provider, err := catalog.NewProvider(item.code, item.code, catalog.APIOpenAI)
		if err != nil {
			t.Fatal(err)
		}
		provider.BaseURL = item.baseURL
		provider.APIKey = "test-key"
		repo.providers[provider.Code] = provider
	}
	service := application.NewProviderService(repo)

	providers, err := service.List(t.Context(), "first")
	if err != nil {
		t.Fatalf("List targeted: %v", err)
	}
	if len(providers) != 2 || len(providers[0].Models)+len(providers[1].Models) != 1 {
		t.Fatalf("targeted providers = %#v", providers)
	}
	if firstRequests.Load() != 1 || secondRequests.Load() != 0 {
		t.Fatalf("targeted requests = (%d, %d), want (1, 0)", firstRequests.Load(), secondRequests.Load())
	}

	if _, err := service.List(t.Context()); err != nil {
		t.Fatalf("List all: %v", err)
	}
	if firstRequests.Load() != 1 || secondRequests.Load() != 1 {
		t.Fatalf("all requests = (%d, %d), want (1, 1)", firstRequests.Load(), secondRequests.Load())
	}
}

func TestProviderUpdate(t *testing.T) {
	_, providerSvc, _ := newServices(t)
	ctx := t.Context()
	if _, err := providerSvc.Create(ctx, "openai", application.ProviderInput{
		Name: "OpenAI", Type: catalog.APIOpenAI, BaseURL: "https://old.example", APIKey: "old-key", FreeFormTool: true,
		Metadata: shared.Metadata{"region": "us"},
	}); err != nil {
		t.Fatal(err)
	}

	updated, err := providerSvc.Update(ctx, "openai", application.ProviderUpdate{
		Name:    ptr("OpenAI Compatible"),
		BaseURL: ptr(""),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "OpenAI Compatible" || updated.BaseURL != "" {
		t.Errorf("updated provider = %+v", updated)
	}
	if updated.APIKey != "old-key" || updated.Type != catalog.APIOpenAI || updated.Metadata["region"] != "us" {
		t.Errorf("unset fields changed: %+v", updated)
	}
	if !updated.FreeFormTool {
		t.Error("freeFormTool = false, want true")
	}

	invalid := catalog.APIType("invalid")
	_, err = providerSvc.Update(ctx, "openai", application.ProviderUpdate{Type: &invalid})
	if code := appErrorCode(err); code != application.CodeValidation {
		t.Errorf("invalid type code = %v, want validation", code)
	}
	anthropic := catalog.APIAnthropic
	updated, err = providerSvc.Update(ctx, "openai", application.ProviderUpdate{Type: &anthropic})
	if err != nil {
		t.Fatalf("change provider type: %v", err)
	}
	if updated.FreeFormTool {
		t.Error("freeFormTool = true after changing to Anthropic, want ignored")
	}
}

func TestBuiltinProviderAllowsOnlyAPIKeyUpdate(t *testing.T) {
	repo := newProviderRepositoryFake()
	provider, err := catalog.NewProvider("openai", "OpenAI", catalog.APIOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	provider.Builtin = true
	provider.Official = true
	provider.BaseURL = "https://api.openai.com/v1"
	provider.Models = []catalog.Model{{Code: "gpt-5-mini", Name: "GPT-5 mini", ContextWindow: 400_000, MaxOutputTokens: 128_000}}
	repo.providers[provider.Code] = provider
	providerSvc := application.NewProviderService(repo)
	ctx := t.Context()
	if _, err := providerSvc.Update(ctx, "openai", application.ProviderUpdate{APIKey: ptr("secret")}); err != nil {
		t.Fatalf("API key update: %v", err)
	}
	updated, err := providerSvc.Get(ctx, "openai")
	if err != nil {
		t.Fatal(err)
	}
	if updated.APIKey != "secret" {
		t.Fatalf("API key = %q", updated.APIKey)
	}

	if _, err := providerSvc.Update(ctx, "openai", application.ProviderUpdate{Name: ptr("Changed")}); appErrorCode(err) != application.CodeValidation {
		t.Fatalf("metadata update error = %v, want validation", err)
	}
	if _, err := providerSvc.Update(ctx, "openai", application.ProviderUpdate{FreeFormTool: ptr(true)}); appErrorCode(err) != application.CodeValidation {
		t.Fatalf("freeFormTool update error = %v, want validation", err)
	}
	if _, err := providerSvc.AddModel(ctx, "openai", "other", application.ModelInput{Name: "Other"}); appErrorCode(err) != application.CodeValidation {
		t.Fatalf("builtin AddModel error = %v, want validation", err)
	}
	if err := providerSvc.Delete(ctx, "openai"); appErrorCode(err) != application.CodeValidation {
		t.Fatalf("builtin Delete error = %v, want validation", err)
	}
}

func TestProviderAddModelAndRemoveModel(t *testing.T) {
	_, providerSvc, _ := newServices(t)
	ctx := context.Background()

	if _, err := providerSvc.Create(ctx, "anthropic", application.ProviderInput{Name: "Anthropic", Type: catalog.APIAnthropic}); err != nil {
		t.Fatal(err)
	}

	p, err := providerSvc.AddModel(ctx, "anthropic", "claude-opus-4-8", application.ModelInput{
		Name:            "Claude Opus 4.8",
		ContextWindow:   200_000,
		MaxOutputTokens: 32_000,
	})
	if err != nil {
		t.Fatalf("AddModel: %v", err)
	}
	if len(p.Models) != 1 {
		t.Fatalf("provider has %d models, want 1", len(p.Models))
	}
	if !p.Models[0].SupportsReasoningEffort(shared.ReasoningMax) {
		t.Error("default reasoning levels do not include max")
	}
	if p.Models[0].MaxOutputTokens != 32_000 {
		t.Errorf("max output tokens = %d, want %d", p.Models[0].MaxOutputTokens, 32_000)
	}

	// AddModel is upsert: re-adding the same code replaces.
	if _, err := providerSvc.AddModel(ctx, "anthropic", "claude-opus-4-8", application.ModelInput{
		Name:            "Claude Opus 4.8 Updated",
		ContextWindow:   210_000,
		MaxOutputTokens: 64_000,
	}); err != nil {
		t.Fatal(err)
	}
	p, err = providerSvc.Get(ctx, "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Models) != 1 {
		t.Errorf("after upsert has %d models, want 1", len(p.Models))
	}
	if p.Models[0].Name != "Claude Opus 4.8 Updated" {
		t.Errorf("model name = %s, want updated", p.Models[0].Name)
	}

	// Model Codes may include provider namespaces, underscores and variant markers.
	const namespacedModelCode = "org/model_name[v2]"
	if _, err := providerSvc.AddModel(ctx, "anthropic", namespacedModelCode, application.ModelInput{
		Name:            "Namespaced model",
		MaxOutputTokens: 16_384,
	}); err != nil {
		t.Fatalf("AddModel namespaced code: %v", err)
	}
	p, err = providerSvc.Get(ctx, "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Model(mustModelCodeForTest(namespacedModelCode)); err != nil {
		t.Fatalf("namespaced model lookup: %v", err)
	}
	if _, err := providerSvc.RemoveModel(ctx, "anthropic", namespacedModelCode); err != nil {
		t.Fatalf("RemoveModel namespaced code: %v", err)
	}

	// Add a second model, then remove the first.
	if _, err := providerSvc.AddModel(ctx, "anthropic", "claude-haiku-4-5", application.ModelInput{
		Name:            "Haiku",
		MaxOutputTokens: 8_000,
		Light:           true,
		Reasoning:       ptr(false),
	}); err != nil {
		t.Fatal(err)
	}
	p, err = providerSvc.RemoveModel(ctx, "anthropic", "claude-opus-4-8")
	if err != nil {
		t.Fatalf("RemoveModel: %v", err)
	}
	if len(p.Models) != 1 {
		t.Fatalf("after remove has %d models, want 1", len(p.Models))
	}
	if p.Models[0].Code.String() != "claude-haiku-4-5" {
		t.Errorf("remaining model = %s, want claude-haiku-4-5", p.Models[0].Code)
	}
	if p.Models[0].SupportsReasoning() {
		t.Error("non-reasoning model supports reasoning")
	}

	// Removing again surfaces not-found.
	_, err = providerSvc.RemoveModel(ctx, "anthropic", "claude-opus-4-8")
	if code := appErrorCode(err); code != application.CodeNotFound {
		t.Errorf("remove missing code = %v, want not_found", code)
	}
}

func mustModelCodeForTest(value string) shared.ModelCode {
	modelCode, err := shared.NewModelCode(value)
	if err != nil {
		panic(err)
	}
	return modelCode
}

func TestProviderAddModelDefaultsReasoningAndAllowsExplicitDisable(t *testing.T) {
	_, providerSvc, _ := newServices(t)
	ctx := t.Context()
	if _, err := providerSvc.Create(ctx, "openai", application.ProviderInput{Name: "OpenAI", Type: catalog.APIOpenAI}); err != nil {
		t.Fatal(err)
	}

	reasoning, err := providerSvc.AddModel(ctx, "openai", "reasoning", application.ModelInput{Name: "Reasoning"})
	if err != nil {
		t.Fatal(err)
	}
	if got := reasoning.Models[0].ReasoningEfforts; !slices.Equal(got, shared.StandardReasoningEfforts()) {
		t.Fatalf("default reasoning efforts = %v", got)
	}

	disabled, err := providerSvc.AddModel(ctx, "openai", "non-reasoning", application.ModelInput{
		Name: "Non-reasoning", Reasoning: ptr(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	model, err := disabled.Model("non-reasoning")
	if err != nil {
		t.Fatal(err)
	}
	if model.ReasoningEfforts == nil || len(model.ReasoningEfforts) != 0 {
		t.Fatalf("disabled reasoning efforts = %v, want empty", model.ReasoningEfforts)
	}
}

func TestProviderAddModelUsesGlobalMaxOutputTokens(t *testing.T) {
	t.Parallel()

	_, providerSvc, _ := newServices(t)
	ctx := t.Context()
	if _, err := providerSvc.Create(ctx, "openai", application.ProviderInput{Name: "OpenAI", Type: catalog.APIOpenAI}); err != nil {
		t.Fatal(err)
	}

	for _, maxOutputTokens := range []int64{0, -1} {
		provider, err := providerSvc.AddModel(ctx, "openai", "gpt-5", application.ModelInput{
			Name:            "GPT-5",
			MaxOutputTokens: maxOutputTokens,
		})
		if err != nil {
			t.Fatalf("maxOutputTokens %d: %v", maxOutputTokens, err)
		}
		if provider.Models[0].MaxOutputTokens != catalog.DefaultMaxOutputTokens {
			t.Errorf("maxOutputTokens %d persisted as %d", maxOutputTokens, provider.Models[0].MaxOutputTokens)
		}
	}
}

func TestProviderDelete(t *testing.T) {
	_, providerSvc, _ := newServices(t)
	ctx := context.Background()

	if _, err := providerSvc.Create(ctx, "anthropic", application.ProviderInput{Name: "Anthropic", Type: catalog.APIAnthropic}); err != nil {
		t.Fatal(err)
	}
	if err := providerSvc.Delete(ctx, "anthropic"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := providerSvc.Get(ctx, "anthropic")
	if code := appErrorCode(err); code != application.CodeNotFound {
		t.Errorf("Get after Delete code = %v, want not_found", code)
	}
}

func TestProviderNotFoundPaths(t *testing.T) {
	_, providerSvc, _ := newServices(t)
	tests := []struct {
		name string
		call func() error
	}{
		{name: "get", call: func() error { _, err := providerSvc.Get(t.Context(), "missing"); return err }},
		{name: "update", call: func() error {
			_, err := providerSvc.Update(t.Context(), "missing", application.ProviderUpdate{})
			return err
		}},
		{name: "delete", call: func() error { return providerSvc.Delete(t.Context(), "missing") }},
		{name: "add model", call: func() error {
			_, err := providerSvc.AddModel(t.Context(), "missing", "model", application.ModelInput{MaxOutputTokens: 1})
			return err
		}},
		{name: "remove model", call: func() error { _, err := providerSvc.RemoveModel(t.Context(), "missing", "model"); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if code := appErrorCode(tt.call()); code != application.CodeNotFound {
				t.Errorf("code = %v, want not_found", code)
			}
		})
	}
}
