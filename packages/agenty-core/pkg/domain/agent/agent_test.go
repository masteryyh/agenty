package agent

import (
	"strings"
	"testing"
)

func TestAgent_ResolveSystemPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		soul               string
		useApplyPatchShell bool
	}{
		{name: "plain text", soul: "You are a coding assistant."},
		{name: "multiline text", soul: "Be direct.\nVerify every change."},
		{name: "special characters", soul: "Use <evidence> & report facts."},
		{name: "empty", soul: ""},
		{
			name:               "apply patch shell fallback",
			soul:               "Edit carefully.",
			useApplyPatchShell: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			agent := &Agent{Soul: tt.soul}
			got, err := agent.ResolveSystemPrompt(SystemPromptOptions{
				UseApplyPatchShell: tt.useApplyPatchShell,
			})
			if err != nil {
				t.Fatalf("ResolveSystemPrompt: %v", err)
			}

			if !strings.Contains(got, "<soul>\n"+tt.soul+"\n</soul>") {
				t.Errorf("ResolveSystemPrompt() soul = %q", got)
			}
			gotApplyPatchShell := strings.Contains(got, "shell tool with one complete apply_patch command")
			if gotApplyPatchShell != tt.useApplyPatchShell {
				t.Errorf(
					"ResolveSystemPrompt() apply_patch shell prompt = %v, want %v",
					gotApplyPatchShell,
					tt.useApplyPatchShell,
				)
			}
			if tt.useApplyPatchShell {
				for _, phrase := range []string{"apply_patch <<'PATCH'", "@'", "'@ | apply_patch", "stdin field", "same file in parallel"} {
					if !strings.Contains(got, phrase) {
						t.Errorf("ResolveSystemPrompt() does not contain %q", phrase)
					}
				}
			}
			if strings.Contains(got, "{{") || strings.Contains(got, "}}") {
				t.Errorf("ResolveSystemPrompt() contains unresolved template actions: %q", got)
			}
		})
	}
}
