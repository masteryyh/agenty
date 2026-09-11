package agentloop_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	infraSkill "github.com/masteryyh/agenty-core/pkg/infra/skill"
)

func TestEngineLoadsExplicitSkillIntoHiddenUserMessage(t *testing.T) {
	root := t.TempDir()
	skillDirectory := filepath.Join(root, "awesome")
	if err := os.MkdirAll(skillDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	location := filepath.Join(skillDirectory, "SKILL.md")
	content := "---\nname: awesome\ndescription: Test skill\n---\n\nFollow this instruction.\n"
	if err := os.WriteFile(location, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	registry, err := infraSkill.ScanRoots([]struct {
		Path   string
		Source string
	}{
		{Path: root, Source: "test"},
	})
	if err != nil {
		t.Fatal(err)
	}

	fixture := newExecutionFixture(t, 8_192)
	caller := &scriptedCaller{responses: []*agentloop.Response{{
		Content:    conversation.Text("done"),
		Usage:      conversation.TokenUsage{Input: 1, Output: 1, Total: 2},
		StopReason: agentloop.StopReasonEndTurn,
	}}}
	engine, err := agentloop.NewEngine(t.Context(), agentloop.Dependencies{
		Sessions: fixture.sessions,
		Catalog:  fixture.catalog,
		Tools:    fixture.registry,
		NewCaller: func(context.Context, catalog.Provider, catalog.Model) (agentloop.Caller, error) {
			return caller, nil
		},
		Skills: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = engine.Shutdown(shutdownCtx)
	})
	session := fixture.createSession(t)
	input := "Use [$awesome](" + location + ") now"
	if _, err := engine.Start(t.Context(), session.ID.String(), conversation.Text(input)); err != nil {
		t.Fatal(err)
	}
	waitForExecution(t, engine, session.ID)
	requests := caller.Requests()
	if len(requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(requests))
	}
	if !strings.Contains(requests[0].SystemPrompt, "<skills>") ||
		!strings.Contains(requests[0].SystemPrompt, "<skill-selection>") {
		t.Fatalf("system prompt does not advertise skills: %q", requests[0].SystemPrompt)
	}
	var found bool
	for _, message := range requests[0].Messages {
		if !message.IsHidden() {
			continue
		}
		if len(message.Content) == 1 {
			if block, ok := message.Content[0].(conversation.TextBlock); ok && block.Text == "<skill-md>\n"+strings.TrimSuffix(content, "\n")+"\n</skill-md>" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("request messages do not contain explicit skill: %#v", requests[0].Messages)
	}
}
