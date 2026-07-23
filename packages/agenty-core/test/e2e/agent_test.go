//go:build e2e

package e2e_test

import "testing"

func TestAgentLifecycle(t *testing.T) {
	t.Parallel()
	core := startCore(t)

	created := decodeResult[agentView](t, core.Call(t, "agent.create", map[string]any{
		"slug":                  "coder",
		"name":                  "Code Assistant",
		"description":           "Builds and reviews code",
		"soul":                  "Prefer clear, verified changes.",
		"defaultModel":          map[string]any{"providerSlug": "openai", "modelSlug": "gpt-5"},
		"defaultContextWindow":  200000,
		"defaultThinkingEffort": "high",
		"isDefault":             true,
		"metadata":              map[string]any{"team": "platform"},
	}))
	if created.Slug != "coder" || created.Name != "Code Assistant" {
		t.Fatalf("created agent = %+v", created)
	}
	if created.DefaultModel == nil || created.DefaultModel.ProviderSlug != "openai" || created.DefaultModel.ModelSlug != "gpt-5" {
		t.Fatalf("created default model = %+v", created.DefaultModel)
	}
	if created.DefaultContextWindow != 200000 || created.DefaultThinkingEffort != "high" || !created.IsDefault {
		t.Fatalf("created defaults = %+v", created)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatal("created agent timestamps must be set")
	}

	requireRPCError(t, core.Call(t, "agent.create", map[string]any{
		"slug": "coder",
		"name": "Duplicate",
	}), errAlreadyExists)

	updated := decodeResult[agentView](t, core.Call(t, "agent.update", map[string]any{
		"slug":        "coder",
		"name":        "Senior Code Assistant",
		"description": "",
		"metadata":    map[string]any{"team": "runtime"},
	}))
	if updated.Name != "Senior Code Assistant" || updated.Description != "" {
		t.Fatalf("updated agent = %+v", updated)
	}
	if updated.Soul != created.Soul || updated.DefaultModel == nil || updated.DefaultModel.ModelSlug != "gpt-5" {
		t.Fatal("partial update changed fields that were not supplied")
	}
	if updated.Metadata["team"] != "runtime" {
		t.Fatalf("updated metadata = %+v", updated.Metadata)
	}

	listed := decodeResult[[]agentView](t, core.Call(t, "agent.list", map[string]any{}))
	if len(listed) != 1 || listed[0].Slug != "coder" {
		t.Fatalf("listed agents = %+v", listed)
	}

	deleted := decodeResult[map[string]any](t, core.Call(t, "agent.delete", map[string]any{"slug": "coder"}))
	if deleted["slug"] != "coder" || deleted["deleted"] != true {
		t.Fatalf("delete result = %+v", deleted)
	}
	requireRPCError(t, core.Call(t, "agent.get", map[string]any{"slug": "coder"}), errNotFound)
}
