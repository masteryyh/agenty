package application_test

import (
	"context"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestAgentCreateAndGet(t *testing.T) {
	agentSvc, _, _ := newServices(t)
	ctx := context.Background()

	a, err := agentSvc.Create(ctx, "coder", application.AgentInput{
		Name:                   "Code Assistant",
		Soul:                   "You are a coder.",
		DefaultContextWindow:   200_000,
		DefaultReasoningEffort: shared.ReasoningHigh,
		IsDefault:              true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.Code.String() != "coder" {
		t.Errorf("code = %s, want coder", a.Code)
	}
	if a.Name != "Code Assistant" {
		t.Errorf("name = %s", a.Name)
	}

	got, err := agentSvc.Get(ctx, "coder")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DefaultContextWindow != 200_000 {
		t.Errorf("context window = %d, want 200000", got.DefaultContextWindow)
	}
	if got.DefaultReasoningEffort != shared.ReasoningHigh {
		t.Errorf("reasoning effort = %s, want high", got.DefaultReasoningEffort)
	}
}

func TestAgentCreateRejectsInvalidReasoningEffort(t *testing.T) {
	agentSvc, _, _ := newServices(t)
	_, err := agentSvc.Create(t.Context(), "coder", application.AgentInput{
		Name:                   "Coder",
		DefaultReasoningEffort: "minimal",
	})
	if code := appErrorCode(err); code != application.CodeValidation {
		t.Errorf("code = %v, want validation", code)
	}
}

func TestAgentCreateDuplicate(t *testing.T) {
	agentSvc, _, _ := newServices(t)
	ctx := context.Background()

	if _, err := agentSvc.Create(ctx, "coder", application.AgentInput{Name: "A"}); err != nil {
		t.Fatal(err)
	}
	_, err := agentSvc.Create(ctx, "coder", application.AgentInput{Name: "B"})
	if code := appErrorCode(err); code != application.CodeAlreadyExists {
		t.Errorf("duplicate Create code = %v, want already_exists", code)
	}
}

func TestAgentCreateInvalidCode(t *testing.T) {
	agentSvc, _, _ := newServices(t)
	_, err := agentSvc.Create(context.Background(), "Bad Code", application.AgentInput{Name: "A"})
	if code := appErrorCode(err); code != application.CodeValidation {
		t.Errorf("invalid code code = %v, want validation", code)
	}
}

func TestAgentList(t *testing.T) {
	agentSvc, _, _ := newServices(t)
	ctx := context.Background()

	for _, code := range []string{"coder", "writer", "reviewer"} {
		if _, err := agentSvc.Create(ctx, code, application.AgentInput{Name: code}); err != nil {
			t.Fatal(err)
		}
	}

	agents, err := agentSvc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(agents) != 3 {
		t.Errorf("List returned %d, want 3", len(agents))
	}
}

func TestAgentUpdate(t *testing.T) {
	agentSvc, _, _ := newServices(t)
	ctx := context.Background()
	model := shared.NewModelRef("anthropic", "claude-opus-4-8")

	if _, err := agentSvc.Create(ctx, "coder", application.AgentInput{
		Name:                   "Old",
		Description:            "old description",
		Soul:                   "old soul",
		DefaultModel:           &model,
		DefaultContextWindow:   200_000,
		DefaultReasoningEffort: shared.ReasoningHigh,
		IsDefault:              true,
		Metadata:               shared.Metadata{"team": "platform"},
	}); err != nil {
		t.Fatal(err)
	}

	updated, err := agentSvc.Update(ctx, "coder", application.AgentUpdate{
		Name:        ptr("New Name"),
		Description: ptr(""),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "New Name" {
		t.Errorf("name = %s, want New Name", updated.Name)
	}
	if updated.Description != "" {
		t.Errorf("description = %q, want explicit clear", updated.Description)
	}
	if updated.Soul != "old soul" || updated.DefaultModel == nil || *updated.DefaultModel != model {
		t.Errorf("unset fields changed: %+v", updated)
	}
	if updated.DefaultContextWindow != 200_000 || updated.DefaultReasoningEffort != shared.ReasoningHigh || !updated.IsDefault {
		t.Errorf("unset defaults changed: %+v", updated)
	}
	if updated.Metadata["team"] != "platform" {
		t.Errorf("metadata = %v, want preserved", updated.Metadata)
	}

	reloaded, err := agentSvc.Get(ctx, "coder")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Name != "New Name" || reloaded.Description != "" {
		t.Errorf("persisted agent = %+v", reloaded)
	}
}

func TestAgentUpdateRejectsInvalidReasoningEffort(t *testing.T) {
	agentSvc, _, _ := newServices(t)
	if _, err := agentSvc.Create(t.Context(), "coder", application.AgentInput{Name: "Coder"}); err != nil {
		t.Fatal(err)
	}

	invalid := shared.ReasoningEffort("minimal")
	_, err := agentSvc.Update(t.Context(), "coder", application.AgentUpdate{DefaultReasoningEffort: &invalid})
	if code := appErrorCode(err); code != application.CodeValidation {
		t.Errorf("code = %v, want validation", code)
	}
}

func TestAgentDelete(t *testing.T) {
	agentSvc, _, _ := newServices(t)
	ctx := context.Background()

	if _, err := agentSvc.Create(ctx, "coder", application.AgentInput{Name: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := agentSvc.Delete(ctx, "coder"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := agentSvc.Get(ctx, "coder")
	if code := appErrorCode(err); code != application.CodeNotFound {
		t.Errorf("Get after Delete code = %v, want not_found", code)
	}
}

func TestAgentGetNotFound(t *testing.T) {
	agentSvc, _, _ := newServices(t)
	_, err := agentSvc.Get(context.Background(), "missing")
	if code := appErrorCode(err); code != application.CodeNotFound {
		t.Errorf("code = %v, want not_found", code)
	}
}

func TestAgentDeleteNotFound(t *testing.T) {
	agentSvc, _, _ := newServices(t)
	err := agentSvc.Delete(t.Context(), "missing")
	if code := appErrorCode(err); code != application.CodeNotFound {
		t.Errorf("code = %v, want not_found", code)
	}
}
