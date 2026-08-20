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
		Code:            mustCatalogModelCode(`org/claude\\claude-opus[fast]`),
		Name:            "Claude Opus 4.8",
		ContextWindow:   200000,
		MaxOutputTokens: 32000,
		ReasoningEffortMapping: map[string]shared.ReasoningEffort{
			"low":    shared.ReasoningLow,
			"medium": shared.ReasoningMedium,
			"high":   shared.ReasoningHigh,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	model2 := catalog.Model{
		Code:            mustCatalogModelCode("claude-haiku-4-5"),
		Name:            "Claude Haiku 4.5",
		ContextWindow:   200000,
		MaxOutputTokens: 8000,
		Light:           true,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	provider.Models = []catalog.Model{model1, model2}

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
	if gotOpus != nil && gotOpus.MaxOutputTokens != catalog.DefaultMaxOutputTokens {
		t.Errorf("opus max output tokens = %d, want %d", gotOpus.MaxOutputTokens, catalog.DefaultMaxOutputTokens)
	}
	if gotHaiku != nil && gotHaiku.SupportsReasoning() {
		t.Errorf("haiku SupportsReasoning = %v, want false", gotHaiku.SupportsReasoning())
	}
	if gotOpus != nil {
		if effort, ok := gotOpus.MapReasoningEffort("medium"); !ok || effort != shared.ReasoningMedium {
			t.Errorf("mapped medium effort = %q, %v; want medium, true", effort, ok)
		}
	}
	if gotHaiku != nil && !gotHaiku.Light {
		t.Errorf("haiku Light = %v, want true", gotHaiku.Light)
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
