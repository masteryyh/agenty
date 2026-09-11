package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	domainmcp "github.com/masteryyh/agenty-core/pkg/domain/mcp"
)

func TestRegistryLoadsSeparateServerFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "mcp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMCPConfig(t, dir, "disabled", domainmcp.Config{
		Type:    domainmcp.TransportStdio,
		Enabled: false,
		Command: "example-server",
	})
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	registry, err := NewRegistry(context.Background(), dir, agentloop.NewRegistry(), Options{})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	defer func() {
		if err := registry.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	}()

	servers := registry.List(context.Background())
	if len(servers) != 2 {
		t.Fatalf("List returned %d servers, want 2: %#v", len(servers), servers)
	}
	if servers[0].Name != "broken" || servers[0].Status != domainmcp.StatusError {
		t.Fatalf("broken server = %#v", servers[0])
	}
	if servers[1].Name != "disabled" || servers[1].Status != domainmcp.StatusDisabled {
		t.Fatalf("disabled server = %#v", servers[1])
	}
	if info, err := os.Stat(filepath.Join(dir, "disabled.json")); err != nil {
		t.Fatalf("Stat disabled config: %v", err)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("loaded config mode = %o, want 600", got)
	}
}

func TestRegistryCRUDPersistsOneFilePerServer(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "mcp")
	registry, err := NewRegistry(context.Background(), dir, agentloop.NewRegistry(), Options{})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	defer func() {
		if err := registry.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	}()

	config := domainmcp.Config{Type: domainmcp.TransportHTTP, Enabled: false, URL: "https://example.com/mcp"}
	if _, err := registry.Create(t.Context(), "example", config); err != nil {
		t.Fatalf("Create: %v", err)
	}
	path := filepath.Join(dir, "example.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var persisted domainmcp.Config
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("decode persisted config: %v", err)
	}
	if persisted.URL != config.URL || persisted.Enabled {
		t.Fatalf("persisted config = %#v, want %#v", persisted, config)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatalf("Stat persisted config: %v", err)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("persisted config mode = %o, want 600", got)
	}

	updated := config
	updated.Enabled = true
	updated.URL = "https://example.com/updated"
	if _, err := registry.Update(t.Context(), "example", updated); err != nil {
		t.Fatalf("Update: %v", err)
	}
	server, ok := registry.Get(t.Context(), "example")
	if !ok || server.Config.URL != updated.URL || server.Status != domainmcp.StatusConnecting {
		t.Fatalf("Get after update = %#v, ok=%v", server, ok)
	}
	if err := registry.Remove(t.Context(), "example"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config file still exists, stat err = %v", err)
	}
}

func TestRegistryPersistsStdioArgumentsAsArray(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "mcp")
	registry, err := NewRegistry(context.Background(), dir, agentloop.NewRegistry(), Options{})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	defer func() {
		if err := registry.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	}()

	config := domainmcp.Config{
		Type:    domainmcp.TransportStdio,
		Enabled: false,
		Command: "node",
		Args:    []string{"server.js", "--label", "value with spaces"},
	}
	if _, err := registry.Create(t.Context(), "stdio", config); err != nil {
		t.Fatalf("Create: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "stdio.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var persisted domainmcp.Config
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("decode persisted config: %v", err)
	}
	if !slices.Equal(persisted.Args, config.Args) {
		t.Fatalf("persisted args = %#v, want %#v", persisted.Args, config.Args)
	}

	server, ok := registry.Get(t.Context(), "stdio")
	if !ok || !slices.Equal(server.Config.Args, config.Args) {
		t.Fatalf("Get config args = %#v, ok=%v, want %#v", server.Config.Args, ok, config.Args)
	}
}

func TestRegistryStartSchedulesConnectionsAsynchronously(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "mcp")
	writeMCPConfig(t, dir, "unreachable", domainmcp.Config{
		Type:    domainmcp.TransportHTTP,
		Enabled: true,
		URL:     "http://127.0.0.1:1/mcp",
	})
	registry, err := NewRegistry(context.Background(), dir, agentloop.NewRegistry(), Options{ConnectTimeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	started := time.Now()
	registry.Start()
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("Start blocked for %s", elapsed)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := registry.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestRegistryHTTPConnectSkipsSubscriptionListener(t *testing.T) {
	var subscriptionRequests atomic.Int32
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "v1"}, nil)
	server.AddTool(&sdkmcp.Tool{
		Name:        "echo",
		Description: "echo",
		InputSchema: map[string]any{"type": "object"},
	}, nil)
	serverHandler := sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return server },
		&sdkmcp.StreamableHTTPOptions{Stateless: true},
	)
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
				return
			}
			request.Body = io.NopCloser(bytes.NewReader(body))
			if bytes.Contains(body, []byte(`"subscriptions/listen"`)) {
				subscriptionRequests.Add(1)
				http.Error(writer, "session not found", http.StatusNotFound)
				return
			}
		}
		serverHandler.ServeHTTP(writer, request)
	}))
	defer httpServer.Close()

	root := t.TempDir()
	dir := filepath.Join(root, "mcp")
	writeMCPConfig(t, dir, "remote", domainmcp.Config{
		Type:    domainmcp.TransportHTTP,
		Enabled: true,
		URL:     httpServer.URL,
	})
	registry, err := NewRegistry(context.Background(), dir, agentloop.NewRegistry(), Options{
		ConnectTimeout: 2 * time.Second,
		ToolTimeout:    2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	defer func() {
		if err := registry.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	}()

	registry.Start()
	deadline := time.Now().Add(2 * time.Second)
	for {
		remote, ok := registry.Get(context.Background(), "remote")
		if ok && remote.Status == domainmcp.StatusConnected {
			if remote.ToolCount != 1 {
				t.Fatalf("ToolCount = %d, want 1", remote.ToolCount)
			}
			break
		}
		if ok && remote.Status == domainmcp.StatusError {
			t.Fatalf("HTTP connection failed: %s", remote.Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for HTTP connection: %#v", remote)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := subscriptionRequests.Load(); got != 0 {
		t.Fatalf("subscriptions/listen requests = %d, want 0", got)
	}
}

func TestNewRemoteToolUsesNamespacedDefinition(t *testing.T) {
	tool, err := newRemoteTool(nil, &sdkmcp.Tool{
		Name:        "search",
		Description: "Search",
		InputSchema: map[string]any{"type": "object"},
	}, "github", time.Second)
	if err != nil {
		t.Fatalf("newRemoteTool: %v", err)
	}
	if got := tool.Definition().Name; got != "mcp__github__search" {
		t.Fatalf("Definition().Name = %q", got)
	}
	if got := tool.remoteName; got != "search" {
		t.Fatalf("remoteName = %q", got)
	}
}

func writeMCPConfig(t *testing.T, dir, name string, config domainmcp.Config) {
	t.Helper()
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
