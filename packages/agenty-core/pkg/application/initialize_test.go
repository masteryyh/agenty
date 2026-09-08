package application_test

import (
	"context"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type initializationStateFake struct {
	initialized bool
	model       shared.ModelRef
	eff         shared.ReasoningEffort
	err         error
}

func (s *initializationStateFake) Initialized() bool { return s.initialized }

func (s *initializationStateFake) SetInitialized(initialized bool) error {
	if s.err != nil {
		return s.err
	}
	s.initialized = initialized
	return nil
}

func (s *initializationStateFake) DefaultModel() (shared.ModelRef, shared.ReasoningEffort) {
	return s.model, s.eff
}

func (s *initializationStateFake) SetDefaultModel(model shared.ModelRef, effort shared.ReasoningEffort) error {
	if s.err != nil {
		return s.err
	}
	s.model, s.eff = model, effort
	return nil
}

func TestInitializeServiceCompletesConfiguredResources(t *testing.T) {
	providers, _ := newServices(t)
	state := &initializationStateFake{}
	svc := application.NewInitializeService(providers, state)
	ctx := context.Background()

	provider, err := providers.Create(ctx, "openai", application.ProviderInput{Name: "OpenAI", Type: catalog.APIOpenAI})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	provider, err = providers.Update(ctx, "openai", application.ProviderUpdate{
		Name: ptr("OpenAI Updated"), Type: ptr(catalog.APIOpenAI), APIKey: ptr("secret"),
	})
	if err != nil {
		t.Fatalf("update provider: %v", err)
	}
	if provider.Name != "OpenAI Updated" || provider.APIKey != "secret" {
		t.Fatalf("provider = %+v", provider)
	}

	provider, err = providers.AddModel(ctx, "openai", "gpt-test", application.ModelInput{
		Name: "GPT Test", ContextWindow: 128_000, MaxOutputTokens: 16_384, IsDefault: true,
	})
	if err != nil {
		t.Fatalf("add model: %v", err)
	}
	if len(provider.Models) != 1 {
		t.Fatalf("models = %d, want 1", len(provider.Models))
	}

	completed, err := svc.Complete(ctx, application.InitializeCompleteInput{
		ProviderCode: "openai", ModelCode: "gpt-test", ReasoningEffort: shared.ReasoningHigh,
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if !completed.Initialized || !state.initialized {
		t.Fatalf("completed = %+v, state = %+v", completed, state)
	}
	if completed.DefaultModel == nil || *completed.DefaultModel != shared.NewModelRef("openai", "gpt-test") {
		t.Fatalf("default model = %+v", completed.DefaultModel)
	}
	if state.eff != shared.ReasoningHigh {
		t.Fatalf("reasoning effort = %q", state.eff)
	}
}

func TestInitializeServiceRejectsMissingModel(t *testing.T) {
	providers, _ := newServices(t)
	state := &initializationStateFake{}
	svc := application.NewInitializeService(providers, state)
	ctx := context.Background()

	if _, err := providers.Create(ctx, "openai", application.ProviderInput{Name: "OpenAI", Type: catalog.APIOpenAI}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Complete(ctx, application.InitializeCompleteInput{
		ProviderCode: "openai", ModelCode: "missing",
	})
	if code := appErrorCode(err); code != application.CodeNotFound {
		t.Fatalf("error code = %v, want not found: %v", code, err)
	}
	if state.initialized {
		t.Fatal("state initialized after failed completion")
	}
}
