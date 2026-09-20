package session_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	infraMiddleware "github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
	infrasession "github.com/masteryyh/agenty-core/pkg/infra/session"
	infraSkill "github.com/masteryyh/agenty-core/pkg/infra/skill"
	infraStorage "github.com/masteryyh/agenty-core/pkg/infra/storage"
)

func TestEngineAppendsSkillPromptToSessionSystemPrompt(t *testing.T) {
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
	middlewareManager := infraMiddleware.NewMiddlewareManager()
	if err := middlewareManager.Register(infraSkill.NewMiddleware(registry)); err != nil {
		t.Fatal(err)
	}
	if err := middlewareManager.Register(infraStorage.NewSessionMiddleware(fixture.sessions)); err != nil {
		t.Fatal(err)
	}
	middlewareChain, err := middlewareManager.Compile()
	if err != nil {
		t.Fatal(err)
	}

	caller := &scriptedCaller{responses: []*modelcall.ModelCallResponse{{
		Content:    conversation.Text("done"),
		Usage:      conversation.TokenUsage{Input: 1, Output: 1, Total: 2},
		StopReason: modelcall.ModelCallStopReasonEndTurn,
	}}}
	engine, err := infrasession.NewEngine(t.Context(), infrasession.Dependencies{
		Sessions:    fixture.sessions,
		Catalog:     fixture.catalog,
		Tools:       fixture.registry,
		InvokeModel: caller.Call,
		Lifecycle:   middlewareChain.LifecycleHooks(),
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
		strings.Contains(requests[0].SystemPrompt, "<skill-selection>") {
		t.Fatalf("system prompt does not advertise skills: %q", requests[0].SystemPrompt)
	}
	for _, message := range requests[0].Messages {
		if len(message.Content) == 1 {
			if block, ok := message.Content[0].(conversation.TextBlock); ok && strings.Contains(block.Text, "<skill-md>") {
				t.Fatalf("request contains an explicit skill message: %#v", message)
			}
		}
	}
}
