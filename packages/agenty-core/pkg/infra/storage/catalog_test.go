package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func newCatalogRepo(t *testing.T) *CatalogRepository {
	t.Helper()
	return NewCatalogRepository(filepath.Join(t.TempDir(), "providers"))
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
		Slug:          mustSlug("claude-opus-4-8"),
		Name:          "Claude Opus 4.8",
		ContextWindow: 200000,
		ThinkingEfforts: []shared.ThinkingEffort{
			shared.ThinkingLow,
			shared.ThinkingMedium,
			shared.ThinkingHigh,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	model2 := catalog.Model{
		Slug:          mustSlug("claude-haiku-4-5"),
		Name:          "Claude Haiku 4.5",
		ContextWindow: 200000,
		Light:         true,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	provider.Models = []catalog.Model{model1, model2}

	if err := repo.Save(ctx, provider); err != nil {
		t.Fatalf("Save: %v", err)
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
	if gotOpus != nil && !gotOpus.SupportsThinking() {
		t.Errorf("opus SupportsThinking = %v, want true", gotOpus.SupportsThinking())
	}
	if gotHaiku != nil && gotHaiku.SupportsThinking() {
		t.Errorf("haiku SupportsThinking = %v, want false", gotHaiku.SupportsThinking())
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
}

func TestCatalogGetReturnsNotFoundWhenMissing(t *testing.T) {
	repo := newCatalogRepo(t)
	_, err := repo.Get(context.Background(), mustSlug("unknown"))
	if err != ErrProviderNotFound {
		t.Errorf("Get() = %v, want ErrProviderNotFound", err)
	}
}
