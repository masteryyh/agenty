package application_test

import (
	"context"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestProviderCreateAndGet(t *testing.T) {
	_, providerSvc, _ := newServices(t)
	ctx := context.Background()

	p, err := providerSvc.Create(ctx, "anthropic", application.ProviderInput{
		Name:    "Anthropic",
		Type:    catalog.APIAnthropic,
		BaseURL: "https://api.anthropic.com",
		APIKey:  "sk-ant-test",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Slug.String() != "anthropic" {
		t.Errorf("slug = %s", p.Slug)
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

	for _, slug := range []string{"anthropic", "openai"} {
		if _, err := providerSvc.Create(ctx, slug, application.ProviderInput{Name: slug, Type: catalog.APIOpenAI}); err != nil {
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

func TestProviderUpdate(t *testing.T) {
	_, providerSvc, _ := newServices(t)
	ctx := t.Context()
	if _, err := providerSvc.Create(ctx, "openai", application.ProviderInput{
		Name: "OpenAI", Type: catalog.APIOpenAI, BaseURL: "https://old.example", APIKey: "old-key",
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

	invalid := catalog.APIType("invalid")
	_, err = providerSvc.Update(ctx, "openai", application.ProviderUpdate{Type: &invalid})
	if code := appErrorCode(err); code != application.CodeValidation {
		t.Errorf("invalid type code = %v, want validation", code)
	}
}

func TestProviderAddModelAndRemoveModel(t *testing.T) {
	_, providerSvc, _ := newServices(t)
	ctx := context.Background()

	if _, err := providerSvc.Create(ctx, "anthropic", application.ProviderInput{Name: "Anthropic", Type: catalog.APIAnthropic}); err != nil {
		t.Fatal(err)
	}

	p, err := providerSvc.AddModel(ctx, "anthropic", "claude-opus-4-8", application.ModelInput{
		Name:          "Claude Opus 4.8",
		ContextWindow: 200_000,
		ThinkingEfforts: []shared.ThinkingEffort{
			shared.ThinkingLow,
			shared.ThinkingHigh,
		},
	})
	if err != nil {
		t.Fatalf("AddModel: %v", err)
	}
	if len(p.Models) != 1 {
		t.Fatalf("provider has %d models, want 1", len(p.Models))
	}

	// AddModel is upsert: re-adding the same slug replaces.
	if _, err := providerSvc.AddModel(ctx, "anthropic", "claude-opus-4-8", application.ModelInput{
		Name:          "Claude Opus 4.8 Updated",
		ContextWindow: 210_000,
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

	// Add a second model, then remove the first.
	if _, err := providerSvc.AddModel(ctx, "anthropic", "claude-haiku-4-5", application.ModelInput{
		Name:  "Haiku",
		Light: true,
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
	if p.Models[0].Slug.String() != "claude-haiku-4-5" {
		t.Errorf("remaining model = %s, want claude-haiku-4-5", p.Models[0].Slug)
	}

	// Removing again surfaces not-found.
	_, err = providerSvc.RemoveModel(ctx, "anthropic", "claude-opus-4-8")
	if code := appErrorCode(err); code != application.CodeNotFound {
		t.Errorf("remove missing code = %v, want not_found", code)
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
			_, err := providerSvc.AddModel(t.Context(), "missing", "model", application.ModelInput{})
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
