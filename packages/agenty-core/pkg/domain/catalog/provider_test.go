package catalog

import (
	"errors"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestProvider_ModelLifecycle(t *testing.T) {
	t.Parallel()

	p := &Provider{Models: []Model{
		{Slug: "model-a", Name: "A", IsDefault: true},
		{Slug: "model-b", Name: "B"},
	}}

	got, err := p.Model("model-b")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "B" {
		t.Errorf("model name = %q, want B", got.Name)
	}

	p.AddModel(Model{Slug: "model-b", Name: "B2", ThinkingEfforts: []shared.ThinkingEffort{shared.ThinkingHigh}})
	if len(p.Models) != 2 {
		t.Fatalf("models = %d, want 2 after upsert", len(p.Models))
	}
	got, err = p.Model("model-b")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "B2" || !got.SupportsThinking() || !got.SupportsEffort(shared.ThinkingHigh) {
		t.Errorf("upserted model = %+v", got)
	}

	defaultModel, ok := p.DefaultModel()
	if !ok || defaultModel.Slug != "model-a" {
		t.Errorf("default model = %+v, %v", defaultModel, ok)
	}

	p.RemoveModel("model-a")
	if _, err := p.Model("model-a"); !errors.Is(err, ErrModelNotFound) {
		t.Errorf("removed model lookup error = %v, want ErrModelNotFound", err)
	}
	if _, ok := p.DefaultModel(); ok {
		t.Error("DefaultModel found a model after the default was removed")
	}
}
