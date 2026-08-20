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
	err         error
}

func (s *initializationStateFake) Initialized() bool {
	return s.initialized
}

func (s *initializationStateFake) SetInitialized(initialized bool) error {
	if s.err != nil {
		return s.err
	}
	s.initialized = initialized
	return nil
}

func TestInitializeServiceCompletesConfiguredResources(t *testing.T) {
	agents, providers, _ := newServices(t)
	state := &initializationStateFake{}
	svc := application.NewInitializeService(agents, providers, state)
	ctx := context.Background()

	if got := svc.Already(ctx); got.Initialized {
		t.Fatal("Already().Initialized = true, want false")
	}
	provider, err := providers.Create(ctx, "openai", application.ProviderInput{Name: "OpenAI", Type: catalog.APIOpenAI})
	if err != nil {
		t.Fatalf("SetProvider create: %v", err)
	}
	provider, err = providers.Update(ctx, "openai", application.ProviderUpdate{
		Name: ptr("OpenAI Updated"), Type: ptr(catalog.APIOpenAI), APIKey: ptr("secret"),
	})
	if err != nil {
		t.Fatalf("SetProvider update: %v", err)
	}
	if provider.Name != "OpenAI Updated" || provider.APIKey != "secret" {
		t.Fatalf("provider = %+v", provider)
	}

	provider, err = providers.AddModel(ctx, "openai", "gpt-test", application.ModelInput{
		Name:            "GPT Test",
		ContextWindow:   128_000,
		MaxOutputTokens: 16_384,
		IsDefault:       true,
	})
	if err != nil {
		t.Fatalf("SetModel: %v", err)
	}
	if len(provider.Models) != 1 {
		t.Fatalf("models = %d, want 1", len(provider.Models))
	}

	modelRef := &shared.ModelRef{ProviderCode: "openai", ModelCode: "gpt-test"}
	agentResult, err := agents.Create(ctx, "default", application.AgentInput{
		Name:                 "Default",
		Soul:                 "Be helpful.",
		DefaultModel:         modelRef,
		DefaultContextWindow: 128_000,
		IsDefault:            true,
	})
	if err != nil {
		t.Fatalf("SetAgent create: %v", err)
	}
	if agentResult.DefaultModel == nil || *agentResult.DefaultModel != *modelRef {
		t.Fatalf("agent default model = %+v", agentResult.DefaultModel)
	}

	completed, err := svc.Complete(ctx, application.InitializeCompleteInput{
		AgentCode:    "default",
		ProviderCode: "openai",
		ModelCode:    "gpt-test",
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !completed.Initialized || !state.initialized {
		t.Fatalf("completed = %+v, state = %+v", completed, state)
	}
}

func TestInitializeServiceRejectsMismatchedAgentModel(t *testing.T) {
	agents, providers, _ := newServices(t)
	state := &initializationStateFake{}
	svc := application.NewInitializeService(agents, providers, state)
	ctx := context.Background()

	_, err := providers.Create(ctx, "openai", application.ProviderInput{Name: "OpenAI", Type: catalog.APIOpenAI})
	if err != nil {
		t.Fatal(err)
	}
	_, err = providers.AddModel(ctx, "openai", "gpt-test", application.ModelInput{
		Name:            "GPT Test",
		MaxOutputTokens: 8_192,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = agents.Create(ctx, "default", application.AgentInput{Name: "Default"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Complete(ctx, application.InitializeCompleteInput{
		AgentCode:    "default",
		ProviderCode: "openai",
		ModelCode:    "gpt-test",
	})
	if code := appErrorCode(err); code != application.CodeValidation {
		t.Fatalf("error code = %v, want validation: %v", code, err)
	}
	if state.initialized {
		t.Fatal("state initialized after failed completion")
	}
}
