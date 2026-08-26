package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-core/pkg/infra/catalogdata"
)

func newCatalogRepo(t *testing.T) *CatalogRepository {
	t.Helper()
	return NewCatalogRepository(filepath.Join(t.TempDir(), "providers"))
}

func mustCatalogModelCode(value string) shared.ModelCode {
	modelCode, err := shared.NewModelCode(value)
	if err != nil {
		panic(err)
	}
	return modelCode
}

func TestCatalogSaveAndGet(t *testing.T) {
	repo := newCatalogRepo(t)
	ctx := context.Background()

	provider, err := catalog.NewProvider("anthropic", "Anthropic", catalog.APIAnthropic)
	if err != nil {
		t.Fatal(err)
	}
	provider.BaseURL = "https://api.anthropic.com"
	provider.APIKey = "sk-ant-test"

	model1 := catalog.Model{
		Code:             mustCatalogModelCode(`org/claude\\claude-opus[fast]`),
		Name:             "Claude Opus 4.8",
		ContextWindow:    200000,
		MaxOutputTokens:  32000,
		ReasoningEfforts: []shared.ReasoningEffort{shared.ReasoningLow, shared.ReasoningMedium, shared.ReasoningHigh},
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
	model2 := catalog.Model{
		Code:             mustCatalogModelCode("claude-haiku-4-5"),
		Name:             "Claude Haiku 4.5",
		ContextWindow:    200000,
		MaxOutputTokens:  8000,
		Light:            true,
		ReasoningEfforts: []shared.ReasoningEffort{},
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
	provider.Models = []catalog.Model{model1, model2}
	provider.ModelsCached = false

	if err := repo.Save(ctx, provider); err != nil {
		t.Fatalf("Save: %v", err)
	}
	providerData, err := os.ReadFile(filepath.Join(repo.providersDir, provider.Code.String()+".json"))
	if err != nil {
		t.Fatalf("read provider file: %v", err)
	}
	var persistedProvider catalog.Provider
	if err := json.Unmarshal(providerData, &persistedProvider); err != nil {
		t.Fatalf("decode provider file: %v", err)
	}
	if persistedProvider.ModelsCached {
		t.Fatal("transient cache marker was persisted in provider config")
	}
	if len(persistedProvider.Models) != 2 {
		t.Fatalf("persisted %d models, want 2", len(persistedProvider.Models))
	}
	if persistedProvider.Models[0].Code != model1.Code && persistedProvider.Models[1].Code != model1.Code {
		t.Errorf("persisted models = %+v, want model %s", persistedProvider.Models, model1.Code)
	}

	loaded, err := repo.Get(ctx, provider.Code)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if loaded.Code != provider.Code {
		t.Errorf("code = %s, want %s", loaded.Code, provider.Code)
	}
	if loaded.Name != provider.Name {
		t.Errorf("name = %s, want %s", loaded.Name, provider.Name)
	}
	if len(loaded.Models) != 2 {
		t.Fatalf("loaded %d models, want 2", len(loaded.Models))
	}

	// Models may load in any order; find by code.
	var gotOpus, gotHaiku *catalog.Model
	for i := range loaded.Models {
		if loaded.Models[i].Code == model1.Code {
			gotOpus = &loaded.Models[i]
		}
		if loaded.Models[i].Code == model2.Code {
			gotHaiku = &loaded.Models[i]
		}
	}
	if gotOpus == nil {
		t.Error("claude-opus-4-8 not found in loaded models")
	}
	if gotHaiku == nil {
		t.Error("claude-haiku-4-5 not found in loaded models")
	}
	if gotOpus != nil && !gotOpus.SupportsReasoning() {
		t.Errorf("opus SupportsReasoning = %v, want true", gotOpus.SupportsReasoning())
	}
	if gotOpus != nil && gotOpus.MaxOutputTokens != model1.MaxOutputTokens {
		t.Errorf("opus max output tokens = %d, want %d", gotOpus.MaxOutputTokens, model1.MaxOutputTokens)
	}
	if gotHaiku != nil && gotHaiku.SupportsReasoning() {
		t.Errorf("haiku SupportsReasoning = %v, want false", gotHaiku.SupportsReasoning())
	}
	if gotOpus != nil && !gotOpus.SupportsReasoningEffort(shared.ReasoningMedium) {
		t.Error("opus does not support medium reasoning effort")
	}
	if gotHaiku != nil && !gotHaiku.Light {
		t.Errorf("haiku Light = %v, want true", gotHaiku.Light)
	}
}

func TestCatalogReasoningModelWithEmptyEffortsUsesDefaults(t *testing.T) {
	repo := newCatalogRepo(t)
	provider, err := catalog.NewProvider("custom", "Custom", catalog.APIOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	provider.Models = []catalog.Model{{
		Code:      mustCatalogModelCode("reasoning-model"),
		Name:      "Reasoning model",
		Reasoning: true,
	}}
	if err := repo.Save(t.Context(), provider); err != nil {
		t.Fatal(err)
	}

	loaded, err := repo.Get(t.Context(), provider.Code)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Models[0].Reasoning || len(loaded.Models[0].ReasoningEfforts) != len(shared.StandardReasoningEfforts()) {
		t.Fatalf("reasoning capabilities = %+v", loaded.Models[0])
	}
}

func TestCatalogBuiltinProviderPersistsOnlyAPIKey(t *testing.T) {
	repo := newCatalogRepo(t)
	builtins, err := catalogdata.LoadProviders()
	if err != nil {
		t.Fatal(err)
	}
	repo = NewCatalogRepository(repo.providersDir, builtins...)
	ctx := context.Background()

	provider, err := repo.Get(ctx, mustCode("openai_legacy"))
	if err != nil {
		t.Fatal(err)
	}
	if !provider.Builtin || provider.Name != "OpenAI (Legacy API)" {
		t.Fatalf("builtin provider = %+v", provider)
	}
	provider.APIKey = "secret"
	if err := repo.Save(ctx, provider); err != nil {
		t.Fatalf("Save builtin: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(repo.providersDir, "openai_legacy.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\n  \"apiKey\": \"secret\"\n}" {
		t.Fatalf("builtin credentials = %s", data)
	}

	loaded, err := repo.Get(ctx, provider.Code)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.APIKey != "secret" || loaded.Name != provider.Name || len(loaded.Models) != len(provider.Models) {
		t.Fatalf("loaded builtin = %+v", loaded)
	}
	for _, model := range loaded.Models {
		if model.ReasoningEfforts == nil {
			t.Errorf("model %s reasoning efforts is nil", model.Code)
		}
	}
	if err := repo.Delete(ctx, provider.Code); err != catalog.ErrBuiltinProviderReadOnly {
		t.Fatalf("Delete builtin = %v, want read-only", err)
	}
}

func TestCatalogModelDiscoveryCacheUsesExpirationAndStaysInMemory(t *testing.T) {
	dir := t.TempDir()
	builtins, err := catalogdata.LoadProviders()
	if err != nil {
		t.Fatal(err)
	}
	repo := NewCatalogRepository(filepath.Join(dir, "providers"), builtins...)
	ctx := context.Background()
	code := mustCode("openrouter")
	models := []catalog.Model{{
		Code:             mustCatalogModelCode("openai/gpt-test"),
		Name:             "GPT Test",
		ContextWindow:    128_000,
		MaxOutputTokens:  16_384,
		ReasoningEfforts: []shared.ReasoningEffort{},
	}}
	expiresAt := time.Now().UTC().Add(catalog.ModelDiscoveryCacheTTL)
	if err := repo.ReplaceModels(ctx, code, models, expiresAt); err != nil {
		t.Fatalf("ReplaceModels: %v", err)
	}

	loaded, err := repo.Get(ctx, code)
	if err != nil {
		t.Fatalf("Get cached provider: %v", err)
	}
	if len(loaded.Models) != 1 || loaded.Models[0].Code != models[0].Code {
		t.Fatalf("cached models = %#v", loaded.Models)
	}
	if !loaded.ModelsCached {
		t.Fatal("cached provider did not expose the transient cache marker")
	}

	cachePath := filepath.Join(repo.providersDir, ".models", "openrouter.json")
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("model discovery cache file exists: %v", err)
	}

	restarted := NewCatalogRepository(repo.providersDir, builtins...)
	restartedProvider, err := restarted.Get(ctx, code)
	if err != nil {
		t.Fatalf("Get after restart: %v", err)
	}
	if len(restartedProvider.Models) != 0 {
		t.Fatalf("restarted models = %#v, want no in-memory cache", restartedProvider.Models)
	}
	if restartedProvider.ModelsCached {
		t.Fatal("restarted provider exposed a cache that should not survive restart")
	}

	if err := repo.ReplaceModels(ctx, code, models, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("ReplaceModels expired: %v", err)
	}
	expired, err := repo.Get(ctx, code)
	if err != nil {
		t.Fatalf("Get expired provider: %v", err)
	}
	if len(expired.Models) != 1 {
		t.Fatalf("expired models = %#v, want stale cache", expired.Models)
	}
	needsDiscovery, err := repo.NeedsModelDiscovery(ctx, code)
	if err != nil {
		t.Fatalf("NeedsModelDiscovery: %v", err)
	}
	if !needsDiscovery {
		t.Fatal("expired cache did not request discovery")
	}
}

func TestCatalogSaveDoesNotPersistDiscoveredModels(t *testing.T) {
	repo := newCatalogRepo(t)
	ctx := context.Background()
	provider, err := catalog.NewProvider("custom", "Custom", catalog.APIOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, provider); err != nil {
		t.Fatal(err)
	}

	model := catalog.Model{
		Code:             mustCatalogModelCode("gateway/model"),
		Name:             "Gateway model",
		ContextWindow:    128_000,
		MaxOutputTokens:  8_192,
		ReasoningEfforts: []shared.ReasoningEffort{},
	}
	if err := repo.ReplaceModels(ctx, provider.Code, []catalog.Model{model}, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	cached, err := repo.Get(ctx, provider.Code)
	if err != nil {
		t.Fatal(err)
	}
	if !cached.ModelsCached || len(cached.Models) != 1 {
		t.Fatalf("cached provider = %+v", cached)
	}

	if err := repo.Save(ctx, cached); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repo.providersDir, "custom.json"))
	if err != nil {
		t.Fatal(err)
	}
	var persisted catalog.Provider
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if len(persisted.Models) != 0 {
		t.Fatalf("persisted discovered models = %#v, want none", persisted.Models)
	}
	reloaded, err := repo.Get(ctx, provider.Code)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.ModelsCached || len(reloaded.Models) != 1 {
		t.Fatalf("cache after Save = %+v", reloaded)
	}
}

func TestCatalogList(t *testing.T) {
	repo := newCatalogRepo(t)
	ctx := context.Background()

	p1, _ := catalog.NewProvider("anthropic", "Anthropic", catalog.APIAnthropic)
	p2, _ := catalog.NewProvider("openai", "OpenAI", catalog.APIOpenAI)

	if err := repo.Save(ctx, p1); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, p2); err != nil {
		t.Fatal(err)
	}

	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("List returned %d providers, want 2", len(all))
	}
}

func TestCatalogListMissingDirectoryReturnsEmptySlice(t *testing.T) {
	repo := newCatalogRepo(t)

	providers, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if providers == nil {
		t.Fatal("List returned nil, want an empty slice")
	}
	if len(providers) != 0 {
		t.Fatalf("List returned %d providers, want zero", len(providers))
	}
}

func TestCatalogDelete(t *testing.T) {
	repo := newCatalogRepo(t)
	ctx := context.Background()

	provider, _ := catalog.NewProvider("anthropic", "Anthropic", catalog.APIAnthropic)
	if err := repo.Save(ctx, provider); err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(ctx, provider.Code); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.Get(ctx, provider.Code)
	if err != ErrProviderNotFound {
		t.Errorf("Get after Delete = %v, want ErrProviderNotFound", err)
	}
	if err := repo.Delete(ctx, provider.Code); err != ErrProviderNotFound {
		t.Errorf("Delete missing provider = %v, want ErrProviderNotFound", err)
	}
}

func TestCatalogGetReturnsNotFoundWhenMissing(t *testing.T) {
	repo := newCatalogRepo(t)
	_, err := repo.Get(context.Background(), mustCode("unknown"))
	if err != ErrProviderNotFound {
		t.Errorf("Get() = %v, want ErrProviderNotFound", err)
	}
}

func TestCatalogSaveAfterRemovingModel(t *testing.T) {
	repo := newCatalogRepo(t)
	ctx := context.Background()

	provider, _ := catalog.NewProvider("anthropic", "Anthropic", catalog.APIAnthropic)
	now := time.Now().UTC()
	provider.Models = []catalog.Model{
		{Code: mustCatalogModelCode("claude-opus-4-8"), Name: "Opus", CreatedAt: now, UpdatedAt: now},
		{Code: mustCatalogModelCode("claude-haiku-4-5"), Name: "Haiku", CreatedAt: now, UpdatedAt: now},
	}
	if err := repo.Save(ctx, provider); err != nil {
		t.Fatal(err)
	}

	provider.RemoveModel(mustCatalogModelCode("claude-haiku-4-5"))
	if err := repo.Save(ctx, provider); err != nil {
		t.Fatalf("Save after remove: %v", err)
	}

	loaded, err := repo.Get(ctx, provider.Code)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Models) != 1 {
		t.Fatalf("loaded %d models, want 1 after delete", len(loaded.Models))
	}
	if loaded.Models[0].Code != mustCatalogModelCode("claude-opus-4-8") {
		t.Errorf("remaining model = %s, want claude-opus-4-8", loaded.Models[0].Code)
	}

	providerPath := filepath.Join(repo.providersDir, provider.Code.String()+".json")
	if _, err := os.Stat(providerPath); err != nil {
		t.Fatalf("provider file after remove: %v", err)
	}
}
