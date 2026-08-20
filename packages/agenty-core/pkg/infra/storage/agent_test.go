package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/agent"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func newAgentRepo(t *testing.T) *AgentRepository {
	t.Helper()
	return NewAgentRepository(filepath.Join(t.TempDir(), "agents"))
}

func TestAgentSaveAndGet(t *testing.T) {
	repo := newAgentRepo(t)
	ctx := context.Background()

	a, err := agent.New("coder", "Code Assistant")
	if err != nil {
		t.Fatal(err)
	}
	a.Soul = "You are a helpful coding assistant."
	modelCode, err := shared.NewModelCode("claude-opus")
	if err != nil {
		t.Fatal(err)
	}
	model := shared.NewModelRef(mustCode("anthropic"), modelCode)
	a.DefaultModel = &model
	a.DefaultContextWindow = 200_000
	a.DefaultReasoningEffort = shared.ReasoningHigh

	if err := repo.Save(ctx, a); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := repo.Get(ctx, a.Code)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if loaded.Code != a.Code {
		t.Errorf("code = %s, want %s", loaded.Code, a.Code)
	}
	if loaded.Name != a.Name {
		t.Errorf("name = %s, want %s", loaded.Name, a.Name)
	}
	if loaded.Soul != a.Soul {
		t.Errorf("soul = %s, want %s", loaded.Soul, a.Soul)
	}
	if loaded.DefaultModel == nil || *loaded.DefaultModel != model {
		t.Errorf("default model = %v, want %v", loaded.DefaultModel, model)
	}
	if loaded.DefaultContextWindow != 200_000 {
		t.Errorf("default context window = %d, want 200000", loaded.DefaultContextWindow)
	}
	if loaded.DefaultReasoningEffort != shared.ReasoningHigh {
		t.Errorf("default reasoning effort = %q, want high", loaded.DefaultReasoningEffort)
	}
}

func TestAgentList(t *testing.T) {
	repo := newAgentRepo(t)
	ctx := context.Background()

	a1, _ := agent.New("coder", "Coder")
	a2, _ := agent.New("writer", "Writer")

	if err := repo.Save(ctx, a1); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, a2); err != nil {
		t.Fatal(err)
	}

	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("List returned %d agents, want 2", len(all))
	}
}

func TestAgentListMissingDirectoryReturnsEmptySlice(t *testing.T) {
	repo := newAgentRepo(t)

	agents, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if agents == nil {
		t.Fatal("List returned nil, want an empty slice")
	}
	if len(agents) != 0 {
		t.Fatalf("List returned %d agents, want zero", len(agents))
	}
}

func TestAgentDelete(t *testing.T) {
	repo := newAgentRepo(t)
	ctx := context.Background()

	a, _ := agent.New("coder", "Coder")
	if err := repo.Save(ctx, a); err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(ctx, a.Code); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.Get(ctx, a.Code)
	if err != ErrAgentNotFound {
		t.Errorf("Get after Delete = %v, want ErrAgentNotFound", err)
	}
}

func TestAgentDefault(t *testing.T) {
	repo := newAgentRepo(t)
	ctx := context.Background()

	a1, _ := agent.New("coder", "Coder")
	a1.IsDefault = false

	a2, _ := agent.New("writer", "Writer")
	a2.IsDefault = true

	if err := repo.Save(ctx, a1); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, a2); err != nil {
		t.Fatal(err)
	}

	def, err := repo.Default(ctx)
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if def.Code != a2.Code {
		t.Errorf("Default returned %s, want %s", def.Code, a2.Code)
	}
}

func TestAgentDefaultReturnsNotFoundWhenNone(t *testing.T) {
	repo := newAgentRepo(t)
	ctx := context.Background()

	a, _ := agent.New("coder", "Coder")
	a.IsDefault = false
	if err := repo.Save(ctx, a); err != nil {
		t.Fatal(err)
	}

	_, err := repo.Default(ctx)
	if err != ErrAgentNotFound {
		t.Errorf("Default() = %v, want ErrAgentNotFound", err)
	}
}

func TestAgentGetReturnsNotFoundWhenMissing(t *testing.T) {
	repo := newAgentRepo(t)
	_, err := repo.Get(context.Background(), mustCode("unknown"))
	if err != ErrAgentNotFound {
		t.Errorf("Get() = %v, want ErrAgentNotFound", err)
	}
}
