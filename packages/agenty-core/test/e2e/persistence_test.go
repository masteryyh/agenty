//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStatePersistsAcrossProcessRestart(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()

	first := startCoreAt(t, dataDir)
	requireSuccess(t, first.Call(t, "agent.create", map[string]any{
		"slug": "persistent-agent", "name": "Persistent Agent",
	}))
	requireSuccess(t, first.Call(t, "provider.create", map[string]any{
		"slug": "persistent-provider", "name": "Persistent Provider", "type": "openai",
	}))
	requireSuccess(t, first.Call(t, "provider.addModel", map[string]any{
		"providerSlug": "persistent-provider", "modelSlug": "persistent-model", "name": "Persistent Model",
	}))
	session := createSession(t, first, "persistent-agent", "persistent-provider", "persistent-model")
	requireSuccess(t, first.Call(t, "session.setTitle", map[string]any{
		"id": session.ID, "title": "Survives restart",
	}))
	first.Close(t)

	for _, relativePath := range []string{
		"config.json",
		"agenty.sqlite",
		filepath.Join("agents", "persistent-agent.json"),
		filepath.Join("providers", "persistent-provider", "provider.json"),
		filepath.Join("providers", "persistent-provider", "models", "persistent-model.json"),
	} {
		if _, err := os.Stat(filepath.Join(dataDir, relativePath)); err != nil {
			t.Fatalf("persisted path %s: %v", relativePath, err)
		}
	}

	transcripts, err := filepath.Glob(filepath.Join(dataDir, "sessions", "*", "*", "*", "*.jsonl"))
	if err != nil {
		t.Fatalf("glob transcripts: %v", err)
	}
	if len(transcripts) != 1 {
		t.Fatalf("transcripts = %v, want one", transcripts)
	}
	transcript, err := os.ReadFile(transcripts[0])
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	lines := bytes.FieldsFunc(transcript, func(r rune) bool { return r == '\n' })
	if len(lines) != 2 {
		t.Fatalf("transcript events = %d, want 2", len(lines))
	}
	for index, line := range lines {
		if !json.Valid(line) {
			t.Fatalf("transcript line %d is not JSON: %q", index+1, line)
		}
	}

	second := startCoreAt(t, dataDir)
	agent := decodeResult[agentView](t, second.Call(t, "agent.get", map[string]any{"slug": "persistent-agent"}))
	provider := decodeResult[providerView](t, second.Call(t, "provider.get", map[string]any{"slug": "persistent-provider"}))
	reloaded := decodeResult[sessionView](t, second.Call(t, "session.get", map[string]any{"id": session.ID}))
	if agent.Name != "Persistent Agent" {
		t.Fatalf("restarted agent = %+v", agent)
	}
	if len(provider.Models) != 1 || provider.Models[0].Slug != "persistent-model" {
		t.Fatalf("restarted provider = %+v", provider)
	}
	if reloaded.Title == nil || *reloaded.Title != "Survives restart" {
		t.Fatalf("restarted session = %+v", reloaded)
	}
}
