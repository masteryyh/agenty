package application_test

import (
	"context"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func newSession(t *testing.T, sessionSvc *application.SessionService, agentSlug string) string {
	t.Helper()
	sess, err := sessionSvc.Create(context.Background(), application.SessionCreateInput{
		AgentSlug:     agentSlug,
		ProviderSlug:  "anthropic",
		ModelSlug:     "claude-opus-4-8",
		ContextWindow: 200_000,
	})
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	return sess.ID.String()
}

func TestSessionCreateAndGet(t *testing.T) {
	_, _, sessionSvc := newServices(t)
	ctx := context.Background()

	sess, err := sessionSvc.Create(ctx, application.SessionCreateInput{
		AgentSlug:      "coder",
		ProviderSlug:   "anthropic",
		ModelSlug:      "claude-opus-4-8",
		ContextWindow:  200_000,
		ThinkingEffort: shared.ThinkingHigh,
		Cwd:            ptr("/tmp/work"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sess.ID.String() == "" {
		t.Error("session id is empty")
	}
	if sess.CurrentModel == nil || sess.CurrentModel.ModelSlug.String() != "claude-opus-4-8" {
		t.Errorf("current model = %+v", sess.CurrentModel)
	}
	if sess.Cwd == nil || *sess.Cwd != "/tmp/work" {
		t.Errorf("cwd = %v, want /tmp/work", sess.Cwd)
	}

	got, err := sessionSvc.Get(ctx, sess.ID.String())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != sess.ID {
		t.Errorf("id = %v, want %v", got.ID, sess.ID)
	}
	if got.CurrentThinkingEffort != shared.ThinkingHigh {
		t.Errorf("thinking effort = %s, want high", got.CurrentThinkingEffort)
	}
}

func TestSessionCreateRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input application.SessionCreateInput
	}{
		{name: "agent slug", input: application.SessionCreateInput{AgentSlug: "Bad Slug", ProviderSlug: "anthropic", ModelSlug: "claude-opus"}},
		{name: "provider slug", input: application.SessionCreateInput{AgentSlug: "coder", ProviderSlug: "Bad Slug", ModelSlug: "claude-opus"}},
		{name: "model slug", input: application.SessionCreateInput{AgentSlug: "coder", ProviderSlug: "anthropic", ModelSlug: "Bad Slug"}},
		{name: "thinking effort", input: application.SessionCreateInput{AgentSlug: "coder", ProviderSlug: "anthropic", ModelSlug: "claude-opus", ThinkingEffort: "extreme"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, sessionSvc := newServices(t)
			_, err := sessionSvc.Create(t.Context(), tt.input)
			if code := appErrorCode(err); code != application.CodeValidation {
				t.Errorf("code = %v, want validation", code)
			}
		})
	}
}

func TestSessionCreateDefaultsThinkingOff(t *testing.T) {
	_, _, sessionSvc := newServices(t)
	sess, err := sessionSvc.Create(t.Context(), application.SessionCreateInput{
		AgentSlug: "coder", ProviderSlug: "anthropic", ModelSlug: "claude-opus",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sess.CurrentThinkingEffort != shared.ThinkingOff {
		t.Errorf("thinking effort = %q, want off", sess.CurrentThinkingEffort)
	}
}

func TestSessionList(t *testing.T) {
	_, _, sessionSvc := newServices(t)
	ctx := context.Background()

	for _, agent := range []string{"coder", "coder", "writer"} {
		newSession(t, sessionSvc, agent)
	}

	all, err := sessionSvc.List(ctx, application.SessionListQuery{})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("List all returned %d, want 3", len(all))
	}

	filtered, err := sessionSvc.List(ctx, application.SessionListQuery{AgentSlug: "coder"})
	if err != nil {
		t.Fatalf("List filtered: %v", err)
	}
	if len(filtered) != 2 {
		t.Errorf("List filtered returned %d, want 2", len(filtered))
	}

	paged, err := sessionSvc.List(ctx, application.SessionListQuery{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("List paged: %v", err)
	}
	if len(paged) != 1 {
		t.Errorf("List paged returned %d, want 1", len(paged))
	}

	if _, err := sessionSvc.List(ctx, application.SessionListQuery{AgentSlug: "Bad Slug"}); appErrorCode(err) != application.CodeValidation {
		t.Errorf("invalid filter error = %v, want validation", err)
	}
}

func TestSessionSetTitle(t *testing.T) {
	_, _, sessionSvc := newServices(t)
	ctx := context.Background()
	id := newSession(t, sessionSvc, "coder")

	if _, err := sessionSvc.SetTitle(ctx, id, "greeting"); err != nil {
		t.Fatalf("SetTitle: %v", err)
	}
	got, err := sessionSvc.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title == nil || *got.Title != "greeting" {
		t.Errorf("title = %v, want greeting", got.Title)
	}
}

func TestSessionSetModel(t *testing.T) {
	_, _, sessionSvc := newServices(t)
	ctx := context.Background()
	id := newSession(t, sessionSvc, "coder")

	if _, err := sessionSvc.SetModel(ctx, id, "openai", "gpt-5.6", 128_000); err != nil {
		t.Fatalf("SetModel: %v", err)
	}
	got, err := sessionSvc.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentModel == nil || got.CurrentModel.ModelSlug.String() != "gpt-5.6" {
		t.Errorf("current model = %+v, want gpt-5.6", got.CurrentModel)
	}
	if got.ContextWindow != 128_000 {
		t.Errorf("context window = %d, want 128000", got.ContextWindow)
	}
}

func TestSessionSetThinkingEffortAndCwd(t *testing.T) {
	_, _, sessionSvc := newServices(t)
	ctx := context.Background()
	id := newSession(t, sessionSvc, "coder")

	if _, err := sessionSvc.SetThinkingEffort(ctx, id, shared.ThinkingMax); err != nil {
		t.Fatalf("SetThinkingEffort: %v", err)
	}
	if _, err := sessionSvc.SetCwd(ctx, id, ptr("/repo")); err != nil {
		t.Fatalf("SetCwd: %v", err)
	}
	got, err := sessionSvc.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentThinkingEffort != shared.ThinkingMax {
		t.Errorf("thinking effort = %s, want max", got.CurrentThinkingEffort)
	}
	if got.Cwd == nil || *got.Cwd != "/repo" {
		t.Errorf("cwd = %v, want /repo", got.Cwd)
	}

	// Clearing cwd with nil.
	if _, err := sessionSvc.SetCwd(ctx, id, nil); err != nil {
		t.Fatalf("SetCwd clear: %v", err)
	}
	got, err = sessionSvc.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cwd != nil {
		t.Errorf("cwd = %v, want nil after clear", got.Cwd)
	}

	if _, err := sessionSvc.SetThinkingEffort(ctx, id, "extreme"); appErrorCode(err) != application.CodeValidation {
		t.Errorf("invalid thinking effort error = %v, want validation", err)
	}
}

func TestSessionDelete(t *testing.T) {
	_, _, sessionSvc := newServices(t)
	ctx := context.Background()
	id := newSession(t, sessionSvc, "coder")

	if err := sessionSvc.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := sessionSvc.Get(ctx, id)
	if code := appErrorCode(err); code != application.CodeNotFound {
		t.Errorf("Get after Delete code = %v, want not_found", code)
	}
}

func TestSessionGetNotFound(t *testing.T) {
	_, _, sessionSvc := newServices(t)
	id := "01957f5e-7c2a-7c2a-9c2a-2c2a2c2a2c2a"
	tests := []struct {
		name string
		call func() error
	}{
		{name: "get", call: func() error { _, err := sessionSvc.Get(t.Context(), id); return err }},
		{name: "delete", call: func() error { return sessionSvc.Delete(t.Context(), id) }},
		{name: "update", call: func() error { _, err := sessionSvc.SetTitle(t.Context(), id, "title"); return err }},
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

func TestSessionRejectsInvalidID(t *testing.T) {
	_, _, sessionSvc := newServices(t)
	tests := []struct {
		name string
		call func() error
	}{
		{name: "get", call: func() error { _, err := sessionSvc.Get(t.Context(), "not-a-uuid"); return err }},
		{name: "delete", call: func() error { return sessionSvc.Delete(t.Context(), "not-a-uuid") }},
		{name: "set title", call: func() error { _, err := sessionSvc.SetTitle(t.Context(), "not-a-uuid", "title"); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if code := appErrorCode(tt.call()); code != application.CodeValidation {
				t.Errorf("code = %v, want validation", code)
			}
		})
	}
}
