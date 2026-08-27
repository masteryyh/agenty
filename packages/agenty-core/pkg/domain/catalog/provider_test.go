package catalog

import (
	"errors"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestProvider_ModelLifecycle(t *testing.T) {
	t.Parallel()

	p := &Provider{Models: []Model{
		{Code: "model-a", Name: "A", IsDefault: true},
		{Code: "model-b", Name: "B"},
	}}

	got, err := p.Model("model-b")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "B" {
		t.Errorf("model name = %q, want B", got.Name)
	}

	p.AddModel(Model{
		Code:             "model-b",
		Name:             "B2",
		ReasoningEfforts: []shared.ReasoningEffort{shared.ReasoningHigh},
	})
	if len(p.Models) != 2 {
		t.Fatalf("models = %d, want 2 after upsert", len(p.Models))
	}
	got, err = p.Model("model-b")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "B2" || !got.SupportsReasoning() || !got.SupportsReasoningEffort(shared.ReasoningHigh) {
		t.Errorf("upserted model = %+v", got)
	}

	defaultModel, ok := p.DefaultModel()
	if !ok || defaultModel.Code != "model-a" {
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

func TestProvider_AddModelSetsSingleDefault(t *testing.T) {
	t.Parallel()

	provider := &Provider{Models: []Model{
		{Code: "model-a", Name: "A", IsDefault: true},
		{Code: "model-b", Name: "B"},
	}}

	provider.AddModel(Model{Code: "model-b", Name: "B", IsDefault: true})

	modelA, err := provider.Model("model-a")
	if err != nil {
		t.Fatal(err)
	}
	modelB, err := provider.Model("model-b")
	if err != nil {
		t.Fatal(err)
	}
	if modelA.IsDefault || !modelB.IsDefault {
		t.Fatalf("default flags = model-a:%t model-b:%t, want false/true", modelA.IsDefault, modelB.IsDefault)
	}

	defaultModel, ok := provider.DefaultModel()
	if !ok || defaultModel.Code != "model-b" {
		t.Fatalf("default model = %+v, %t", defaultModel, ok)
	}
}
