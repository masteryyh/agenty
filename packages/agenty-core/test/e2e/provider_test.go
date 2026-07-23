//go:build e2e

package e2e_test

import "testing"

func TestProviderAndModelLifecycle(t *testing.T) {
	t.Parallel()
	core := startCore(t)

	created := decodeResult[providerView](t, core.Call(t, "provider.create", map[string]any{
		"slug":     "openai",
		"name":     "OpenAI",
		"type":     "openai",
		"baseUrl":  "https://api.example.test/v1",
		"apiKey":   "test-key",
		"metadata": map[string]any{"region": "test"},
	}))
	if created.Slug != "openai" || created.Type != "openai" || created.BaseURL == "" {
		t.Fatalf("created provider = %+v", created)
	}

	requireRPCError(t, core.Call(t, "provider.create", map[string]any{
		"slug": "invalid-provider",
		"name": "Invalid",
		"type": "unsupported",
	}), errInvalidParams)

	updated := decodeResult[providerView](t, core.Call(t, "provider.update", map[string]any{
		"slug":   "openai",
		"name":   "OpenAI Compatible",
		"apiKey": "",
	}))
	if updated.Name != "OpenAI Compatible" || updated.APIKey != "" || updated.BaseURL != created.BaseURL {
		t.Fatalf("updated provider = %+v", updated)
	}

	withModel := decodeResult[providerView](t, core.Call(t, "provider.addModel", map[string]any{
		"providerSlug":    "openai",
		"modelSlug":       "gpt-5",
		"name":            "GPT-5",
		"contextWindow":   200000,
		"multiModal":      true,
		"thinkingEfforts": []string{"low", "high"},
		"isDefault":       true,
	}))
	if len(withModel.Models) != 1 || withModel.Models[0].Slug != "gpt-5" || !withModel.Models[0].IsDefault {
		t.Fatalf("provider models = %+v", withModel.Models)
	}

	upserted := decodeResult[providerView](t, core.Call(t, "provider.addModel", map[string]any{
		"providerSlug":  "openai",
		"modelSlug":     "gpt-5",
		"name":          "GPT-5 Updated",
		"contextWindow": 256000,
		"light":         true,
	}))
	if len(upserted.Models) != 1 || upserted.Models[0].Name != "GPT-5 Updated" || upserted.Models[0].ContextWindow != 256000 {
		t.Fatalf("upserted models = %+v", upserted.Models)
	}

	withoutModel := decodeResult[providerView](t, core.Call(t, "provider.removeModel", map[string]any{
		"providerSlug": "openai",
		"modelSlug":    "gpt-5",
	}))
	if len(withoutModel.Models) != 0 {
		t.Fatalf("models after remove = %+v", withoutModel.Models)
	}
	reloaded := decodeResult[providerView](t, core.Call(t, "provider.get", map[string]any{"slug": "openai"}))
	if len(reloaded.Models) != 0 {
		t.Fatalf("removed model reappeared after reload: %+v", reloaded.Models)
	}
	requireRPCError(t, core.Call(t, "provider.removeModel", map[string]any{
		"providerSlug": "openai",
		"modelSlug":    "gpt-5",
	}), errNotFound)

	deleted := decodeResult[map[string]any](t, core.Call(t, "provider.delete", map[string]any{"slug": "openai"}))
	if deleted["slug"] != "openai" || deleted["deleted"] != true {
		t.Fatalf("delete result = %+v", deleted)
	}
	requireRPCError(t, core.Call(t, "provider.get", map[string]any{"slug": "openai"}), errNotFound)
}
