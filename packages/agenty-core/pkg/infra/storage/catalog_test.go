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

func mustCatalogModelID(value string) shared.ModelID {
	modelID, err := shared.NewModelID(value)
	if err != nil {
		panic(err)
	}
	return modelID
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
		Slug:            mustCatalogModelID("org/claude-opus[fast]"),
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
		Slug:            mustCatalogModelID("claude-haiku-4-5"),
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
	modelData, err := os.ReadFile(filepath.Join(repo.providersDir, provider.Slug.String(), "models", modelFileName(model1.Slug)))
	if err != nil {
		t.Fatalf("read model file: %v", err)
	}
	var persistedModel map[string]shared.RawJSON
	if err := json.Unmarshal(modelData, &persistedModel); err != nil {
		t.Fatalf("decode model file: %v", err)
	}
	if _, ok := persistedModel["reasoningEffortMapping"]; !ok {
		t.Errorf("persisted model keys = %v, want reasoningEffortMapping", persistedModel)
	}
	if _, ok := persistedModel["maxOutputTokens"]; !ok {
		t.Errorf("persisted model keys = %v, want maxOutputTokens", persistedModel)
	}

	loaded, err := repo.Get(ctx, provider.Slug)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if loaded.Slug != provider.Slug {
		t.Errorf("slug = %s, want %s", loaded.Slug, provider.Slug)
	}
	if loaded.Name != provider.Name {
		t.Errorf("name = %s, want %s", loaded.Name, provider.Name)
	}
	if len(loaded.Models) != 2 {
		t.Fatalf("loaded %d models, want 2", len(loaded.Models))
	}

	// Models may load in any order; find by slug.
	var gotOpus, gotHaiku *catalog.Model
	for i := range loaded.Models {
		if loaded.Models[i].Slug == model1.Slug {
			gotOpus = &loaded.Models[i]
		}
		if loaded.Models[i].Slug == model2.Slug {
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
	if gotOpus != nil && gotOpus.MaxOutputTokens != 32000 {
		t.Errorf("opus max output tokens = %d, want 32000", gotOpus.MaxOutputTokens)
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

	if err := repo.Delete(ctx, provider.Slug); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.Get(ctx, provider.Slug)
	if err != ErrProviderNotFound {
		t.Errorf("Get after Delete = %v, want ErrProviderNotFound", err)
	}
	if err := repo.Delete(ctx, provider.Slug); err != ErrProviderNotFound {
		t.Errorf("Delete missing provider = %v, want ErrProviderNotFound", err)
	}
}

func TestCatalogGetReturnsNotFoundWhenMissing(t *testing.T) {
	repo := newCatalogRepo(t)
	_, err := repo.Get(context.Background(), mustSlug("unknown"))
	if err != ErrProviderNotFound {
		t.Errorf("Get() = %v, want ErrProviderNotFound", err)
	}
}

func TestCatalogDeleteModel(t *testing.T) {
	repo := newCatalogRepo(t)
	ctx := context.Background()

	provider, _ := catalog.NewProvider("anthropic", "Anthropic", catalog.APIAnthropic)
	now := time.Now().UTC()
	provider.Models = []catalog.Model{
		{Slug: mustCatalogModelID("claude-opus-4-8"), Name: "Opus", CreatedAt: now, UpdatedAt: now},
		{Slug: mustCatalogModelID("claude-haiku-4-5"), Name: "Haiku", CreatedAt: now, UpdatedAt: now},
	}
	if err := repo.Save(ctx, provider); err != nil {
		t.Fatal(err)
	}

	if err := repo.DeleteModel(ctx, provider.Slug, mustCatalogModelID("claude-haiku-4-5")); err != nil {
		t.Fatalf("DeleteModel: %v", err)
	}

	loaded, err := repo.Get(ctx, provider.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Models) != 1 {
		t.Fatalf("loaded %d models, want 1 after delete", len(loaded.Models))
	}
	if loaded.Models[0].Slug != mustCatalogModelID("claude-opus-4-8") {
		t.Errorf("remaining model = %s, want claude-opus-4-8", loaded.Models[0].Slug)
	}

	// Deleting the same model again surfaces model-not-found, not a silent no-op.
	if err := repo.DeleteModel(ctx, provider.Slug, mustCatalogModelID("claude-haiku-4-5")); err != catalog.ErrModelNotFound {
		t.Errorf("DeleteModel missing model = %v, want catalog.ErrModelNotFound", err)
	}

	// Deleting from a missing provider surfaces provider-not-found.
	if err := repo.DeleteModel(ctx, mustSlug("nope"), mustCatalogModelID("x")); err != ErrProviderNotFound {
		t.Errorf("DeleteModel missing provider = %v, want ErrProviderNotFound", err)
	}
}
