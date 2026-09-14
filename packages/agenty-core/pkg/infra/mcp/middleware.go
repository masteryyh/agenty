package mcp

import (
	"context"
	"sort"

	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	infraMiddleware "github.com/masteryyh/agenty-core/pkg/infra/middleware"
	infratools "github.com/masteryyh/agenty-core/pkg/infra/tools"
)

// NewMiddleware makes the Registry's current immutable tool view part of each
// round. Connections and refreshes remain managed by Registry itself.
func NewMiddleware(registry *Registry) infraMiddleware.Middleware {
	middleware := infraMiddleware.Middleware{Name: "mcp"}
	if registry == nil {
		return middleware
	}

	middleware.BeforeRound = func(_ context.Context, state *infraMiddleware.RoundContext) error {
		if state == nil || state.Tools == nil {
			return nil
		}
		mcpTools := registry.SnapshotToolRuntime()
		*state.Tools = infratools.Combine(*state.Tools, mcpTools)
		return nil
	}

	return middleware
}

func (registry *Registry) SnapshotToolRuntime() agentloop.ToolRuntime {
	if registry == nil {
		return nil
	}

	registry.mu.RLock()
	defer registry.mu.RUnlock()

	// Copy the current server entries into a private registry. Existing remote
	// tool objects retain their session binding, while later registry changes
	// only affect snapshots created by later rounds.
	snapshot := infratools.NewRegistry()
	serverKeys := make([]string, 0, len(registry.servers))
	for key := range registry.servers {
		serverKeys = append(serverKeys, key)
	}
	sort.Strings(serverKeys)
	for _, key := range serverKeys {
		entry := registry.servers[key]
		if entry == nil || !entry.config.Enabled {
			continue
		}
		names := make([]string, 0, len(entry.tools))
		for name := range entry.tools {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			tool := entry.tools[name]
			if tool == nil {
				continue
			}
			// Conflicts are rejected when a server is installed. Keep the
			// snapshot best-effort if a legacy in-memory entry violates that rule.
			_ = snapshot.Register(tool)
		}
	}
	return snapshot.SnapshotToolRuntime()
}
