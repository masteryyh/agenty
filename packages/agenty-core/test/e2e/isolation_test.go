//go:build e2e

package e2e_test

import "testing"

func TestParallelProcessesUseIsolatedDataDirectories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		agentName string
	}{
		{name: "first process", agentName: "First Isolated Agent"},
		{name: "second process", agentName: "Second Isolated Agent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			core := startCore(t)
			created := decodeResult[agentView](t, core.Call(t, "agent.create", map[string]any{
				"slug": "same-slug", "name": tt.agentName,
			}))
			listed := decodeResult[[]agentView](t, core.Call(t, "agent.list", map[string]any{}))
			if created.Name != tt.agentName || len(listed) != 1 || listed[0].Name != tt.agentName {
				t.Fatalf("isolated state = created %+v, listed %+v", created, listed)
			}
		})
	}
}
