//go:build integration

package initialize

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/agent"
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-core/pkg/infra/config"
)

func TestOpenRepositoriesEndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGENTY_DATA_DIR", tmpDir)
	config.ResetForTesting()

	repos, err := OpenRepositories(context.Background())
	if err != nil {
		t.Fatalf("OpenRepositories: %v", err)
	}
	defer repos.Close()

	ctx := context.Background()

	// Directory structure, config and SQLite database were created.
	for _, dir := range []string{"sessions", "agents", "providers"} {
		path := filepath.Join(tmpDir, dir)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected directory %s to exist", path)
		}
	}
	configPath := filepath.Join(tmpDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("expected config.json to exist")
	}
	dbPath := filepath.Join(tmpDir, "agenty.sqlite")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("expected agenty.sqlite to exist")
	}

	// Create and persist the catalog and an agent with its default session model.
	provider, err := catalog.NewProvider("anthropic", "Anthropic", catalog.APIAnthropic)
	if err != nil {
		t.Fatal(err)
	}
	provider.AddModel(catalog.Model{
		Slug:          mustSlug("claude-opus-4-8"),
		Name:          "Claude Opus 4.8",
		ContextWindow: 200_000,
	})
	if err := repos.Catalog.Save(ctx, provider); err != nil {
		t.Fatalf("Save provider: %v", err)
	}

	modelRef := shared.NewModelRef(provider.Slug, mustSlug("claude-opus-4-8"))
	a, err := agent.New("coder", "Code Assistant")
	if err != nil {
		t.Fatal(err)
	}
	a.DefaultModel = &modelRef
	a.DefaultContextWindow = 200_000
	a.DefaultThinkingEffort = shared.ThinkingHigh
	if err := repos.Agent.Save(ctx, a); err != nil {
		t.Fatalf("Save agent: %v", err)
	}

	loadedProvider, err := repos.Catalog.Get(ctx, provider.Slug)
	if err != nil {
		t.Fatalf("Get provider: %v", err)
	}
	if len(loadedProvider.Models) != 1 {
		t.Errorf("loaded %d models, want 1", len(loadedProvider.Models))
	}
	loadedAgent, err := repos.Agent.Get(ctx, a.Slug)
	if err != nil {
		t.Fatalf("Get agent: %v", err)
	}
	if loadedAgent.DefaultModel == nil || *loadedAgent.DefaultModel != modelRef {
		t.Errorf("loaded default model = %v, want %v", loadedAgent.DefaultModel, modelRef)
	}
	if loadedAgent.DefaultContextWindow != 200_000 || loadedAgent.DefaultThinkingEffort != shared.ThinkingHigh {
		t.Errorf("loaded default session config = %+v", loadedAgent)
	}

	// Conversation flow: the application layer resolves the agent's default
	// configuration before constructing a session.
	session := conversation.StartSession(loadedAgent.Slug, *loadedAgent.DefaultModel, loadedAgent.DefaultContextWindow, loadedAgent.DefaultThinkingEffort, nil)
	roundID, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendUserMessage(roundID, conversation.Text("hello")); err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendAssistantMessage(roundID, conversation.Text("hi"), modelRef, &conversation.TokenUsage{Total: 20}); err != nil {
		t.Fatal(err)
	}
	if err := session.CompleteRound(roundID, conversation.RoundCompleted, conversation.TokenUsage{Total: 20}, nil); err != nil {
		t.Fatal(err)
	}
	session.SetTitle("greeting")

	if err := repos.Conversation.Save(ctx, session); err != nil {
		t.Fatalf("Save: %v", err)
	}
	session.ClearPending()

	loaded, err := repos.Conversation.Load(ctx, session.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ID != session.ID {
		t.Errorf("loaded ID = %v, want %v", loaded.ID, session.ID)
	}
	if len(loaded.Rounds) != 1 {
		t.Errorf("loaded %d rounds, want 1", len(loaded.Rounds))
	} else if got := loaded.Rounds[0]; got.Model != modelRef {
		t.Errorf("loaded round configuration = %+v", got)
	}

	summaries, err := repos.Conversation.List(ctx, conversation.ListQuery{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(summaries) != 1 {
		t.Errorf("List returned %d summaries, want 1", len(summaries))
	}
	if summaries[0].Title != "greeting" {
		t.Errorf("summary title = %q, want greeting", summaries[0].Title)
	}
	if summaries[0].ContextWindow != 200_000 {
		t.Errorf("summary context window = %d, want 200000", summaries[0].ContextWindow)
	}
}

func mustSlug(s string) shared.Slug {
	slug, err := shared.NewSlug(s)
	if err != nil {
		panic(err)
	}
	return slug
}
