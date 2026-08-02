package agent

import (
	"strings"
	"testing"
)

func TestAgent_ResolveSystemPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		soul string
	}{
		{name: "plain text", soul: "You are a coding assistant."},
		{name: "multiline text", soul: "Be direct.\nVerify every change."},
		{name: "special characters", soul: "Use <evidence> & report facts."},
		{name: "empty", soul: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			agent := &Agent{Soul: tt.soul}
			got, err := agent.ResolveSystemPrompt()
			if err != nil {
				t.Fatalf("ResolveSystemPrompt: %v", err)
			}

			want := strings.Replace(BaseSystemPrompt, "{{ .Soul }}", tt.soul, 1)
			if got != want {
				t.Errorf("ResolveSystemPrompt() = %q, want %q", got, want)
			}
		})
	}
}
