//go:build e2e

package e2e_test

import "testing"

func TestSessionLifecycleAndListing(t *testing.T) {
	t.Parallel()
	core := startCore(t)

	primary := createSession(t, core, "coder", "openai", "gpt-5")
	if primary.ID == "" || primary.AgentSlug != "coder" || primary.CurrentReasoningEffort != "off" {
		t.Fatalf("created session = %+v", primary)
	}
	if primary.CurrentModel == nil || primary.CurrentModel.ProviderSlug != "openai" || primary.CurrentModel.ModelSlug != "gpt-5" {
		t.Fatalf("created session model = %+v", primary.CurrentModel)
	}

	titled := decodeResult[sessionView](t, core.Call(t, "session.setTitle", map[string]any{
		"id": primary.ID, "title": "Investigate the runtime",
	}))
	if titled.Title == nil || *titled.Title != "Investigate the runtime" {
		t.Fatalf("session title = %+v", titled.Title)
	}

	modeled := decodeResult[sessionView](t, core.Call(t, "session.setModel", map[string]any{
		"id": primary.ID, "providerSlug": "anthropic", "modelSlug": "claude-opus", "contextWindow": 180000,
	}))
	if modeled.CurrentModel == nil || modeled.CurrentModel.ProviderSlug != "anthropic" || modeled.ContextWindow != 180000 {
		t.Fatalf("updated session model = %+v", modeled)
	}

	reasoning := decodeResult[sessionView](t, core.Call(t, "session.setReasoningEffort", map[string]any{
		"id": primary.ID, "reasoningEffort": "high",
	}))
	if reasoning.CurrentReasoningEffort != "high" {
		t.Fatalf("reasoning effort = %q", reasoning.CurrentReasoningEffort)
	}

	working := decodeResult[sessionView](t, core.Call(t, "session.setCwd", map[string]any{
		"id": primary.ID, "cwd": "/tmp/agenty-e2e-workspace",
	}))
	if working.Cwd == nil || *working.Cwd != "/tmp/agenty-e2e-workspace" {
		t.Fatalf("session cwd = %+v", working.Cwd)
	}
	cleared := decodeResult[sessionView](t, core.Call(t, "session.setCwd", map[string]any{
		"id": primary.ID, "cwd": nil,
	}))
	if cleared.Cwd != nil {
		t.Fatalf("cleared cwd = %+v", cleared.Cwd)
	}

	createSession(t, core, "coder", "openai", "gpt-5-mini")
	createSession(t, core, "writer", "openai", "gpt-5")
	filtered := decodeResult[[]sessionSummaryView](t, core.Call(t, "session.list", map[string]any{
		"agentSlug": "coder", "limit": 1, "offset": 1,
	}))
	if len(filtered) != 1 || filtered[0].AgentSlug != "coder" {
		t.Fatalf("filtered sessions = %+v", filtered)
	}

	got := decodeResult[sessionView](t, core.Call(t, "session.get", map[string]any{"id": primary.ID}))
	if got.Title == nil || *got.Title != "Investigate the runtime" || got.CurrentReasoningEffort != "high" {
		t.Fatalf("reloaded session = %+v", got)
	}

	requireSuccess(t, core.Call(t, "session.delete", map[string]any{"id": primary.ID}))
	requireRPCError(t, core.Call(t, "session.get", map[string]any{"id": primary.ID}), errNotFound)
	requireRPCError(t, core.Call(t, "session.get", map[string]any{"id": "not-a-uuid"}), errInvalidParams)
}

func createSession(t *testing.T, core *coreProcess, agentSlug, providerSlug, modelSlug string) sessionView {
	t.Helper()
	return decodeResult[sessionView](t, core.Call(t, "session.create", map[string]any{
		"agentSlug":     agentSlug,
		"providerSlug":  providerSlug,
		"modelSlug":     modelSlug,
		"contextWindow": 128000,
	}))
}
