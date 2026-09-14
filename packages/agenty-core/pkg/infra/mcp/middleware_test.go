package mcp

import (
	"testing"
	"time"

	domainmcp "github.com/masteryyh/agenty-core/pkg/domain/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSnapshotToolRuntimeKeepsTheRoundToolSetStable(t *testing.T) {
	first, err := newRemoteTool(nil, &sdkmcp.Tool{
		Name:        "first",
		InputSchema: map[string]any{"type": "object"},
	}, "remote", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newRemoteTool(nil, &sdkmcp.Tool{
		Name:        "second",
		InputSchema: map[string]any{"type": "object"},
	}, "remote", time.Second)
	if err != nil {
		t.Fatal(err)
	}

	entry := &serverEntry{
		name:   "remote",
		config: domainmcp.Config{Enabled: true},
		tools:  map[string]*remoteTool{first.exposedName: first},
	}
	registry := &Registry{servers: map[string]*serverEntry{"remote": entry}}
	snapshot := registry.SnapshotToolRuntime()

	registry.mu.Lock()
	entry.tools = map[string]*remoteTool{second.exposedName: second}
	registry.mu.Unlock()

	definitions := snapshot.Definitions()
	if len(definitions) != 1 || definitions[0].Name != first.exposedName {
		t.Fatalf("snapshot definitions = %#v, want %q", definitions, first.exposedName)
	}
	current := registry.SnapshotToolRuntime().Definitions()
	if len(current) != 1 || current[0].Name != second.exposedName {
		t.Fatalf("current definitions = %#v, want %q", current, second.exposedName)
	}
}
