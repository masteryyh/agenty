package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"

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

func TestRegistryServerNamesAreCaseInsensitive(t *testing.T) {
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
	server, err := registry.Create(t.Context(), "GitHub", config)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if server.Name != "GitHub" {
		t.Fatalf("created server name = %q, want preserved casing", server.Name)
	}
	if _, ok := registry.Get(t.Context(), "github"); !ok {
		t.Fatal("Get did not match server name case-insensitively")
	}
	if _, err := registry.Create(t.Context(), "github", config); err == nil {
		t.Fatal("Create accepted a case-insensitive duplicate")
	} else {
		var registryErr *RegistryError
		if !errors.As(err, &registryErr) || registryErr.Kind != RegistryErrorAlreadyExists {
			t.Fatalf("duplicate error = %v, want already-exists registry error", err)
		}
	}
	if err := registry.Remove(t.Context(), "github"); err != nil {
		t.Fatalf("Remove with different casing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "GitHub.json")); !os.IsNotExist(err) {
		t.Fatalf("preserved-casing config still exists, stat err = %v", err)
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
	if !tool.Definition().Destructive {
		t.Fatal("missing annotations should classify the tool as destructive")
	}
}

func TestNewRemoteToolMapsReadOnlyAnnotation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		annotations     *sdkmcp.ToolAnnotations
		wantDestructive bool
	}{
		{name: "missing annotations", wantDestructive: true},
		{
			name: "read only",
			annotations: &sdkmcp.ToolAnnotations{
				ReadOnlyHint: true,
			},
			wantDestructive: false,
		},
		{
			name: "read only takes precedence",
			annotations: &sdkmcp.ToolAnnotations{
				DestructiveHint: mcpBoolPointer(true),
				ReadOnlyHint:    true,
			},
			wantDestructive: false,
		},
		{
			name: "additive hint still permits writes",
			annotations: &sdkmcp.ToolAnnotations{
				DestructiveHint: mcpBoolPointer(false),
			},
			wantDestructive: true,
		},
		{
			name: "destructive hint",
			annotations: &sdkmcp.ToolAnnotations{
				DestructiveHint: mcpBoolPointer(true),
			},
			wantDestructive: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			tool, err := newRemoteTool(nil, &sdkmcp.Tool{
				Name:        "search",
				Annotations: test.annotations,
				InputSchema: map[string]any{"type": "object"},
			}, "github", time.Second)
			if err != nil {
				t.Fatalf("newRemoteTool: %v", err)
			}
			if got := tool.Definition().Destructive; got != test.wantDestructive {
				t.Fatalf("destructive = %t, want %t", got, test.wantDestructive)
			}
		})
	}
}

func mcpBoolPointer(value bool) *bool {
	return &value
}

func TestNewRemoteToolNormalizesAndTruncatesProviderName(t *testing.T) {
	tool, err := newRemoteTool(nil, &sdkmcp.Tool{
		Name:        "search/v2 now",
		Description: "Search",
		InputSchema: map[string]any{"type": "object"},
	}, "GitHub", time.Second)
	if err != nil {
		t.Fatalf("newRemoteTool: %v", err)
	}
	if got := tool.Definition().Name; got != "mcp__GitHub__search_v2_now" {
		t.Fatalf("normalized tool name = %q", got)
	}

	long, err := newRemoteTool(nil, &sdkmcp.Tool{
		Name:        strings.Repeat("x", 100),
		InputSchema: map[string]any{"type": "object"},
	}, "github", time.Second)
	if err != nil {
		t.Fatalf("newRemoteTool long name: %v", err)
	}
	if got := long.Definition().Name; len(got) != maxProviderToolNameLength {
		t.Fatalf("truncated tool name length = %d, want %d", len(got), maxProviderToolNameLength)
	}
}

func TestRemoteToolNameCollisionsAreRejectedBeforeInstall(t *testing.T) {
	first, err := newRemoteTool(nil, &sdkmcp.Tool{
		Name:        "foo.bar",
		InputSchema: map[string]any{"type": "object"},
	}, "remote", time.Second)
	if err != nil {
		t.Fatalf("first tool: %v", err)
	}
	second, err := newRemoteTool(nil, &sdkmcp.Tool{
		Name:        "foo_bar",
		InputSchema: map[string]any{"type": "object"},
	}, "remote", time.Second)
	if err != nil {
		t.Fatalf("second tool: %v", err)
	}
	if err := ensureUniqueRemoteToolNames([]*remoteTool{first, second}); err == nil {
		t.Fatal("ensureUniqueRemoteToolNames accepted a normalized collision")
	}
}

func TestInstallRejectsRemoteToolNameCollisionsBeforeMutation(t *testing.T) {
	registry, err := NewRegistry(context.Background(), filepath.Join(t.TempDir(), "mcp"), agentloop.NewRegistry(), Options{})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	defer func() {
		if err := registry.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	}()

	registry.mu.Lock()
	registry.servers["remote"] = &serverEntry{
		name: "remote",
		config: domainmcp.Config{
			Type:    domainmcp.TransportHTTP,
			Enabled: true,
			URL:     "https://example.com/mcp",
		},
		tools: make(map[string]*remoteTool),
	}
	registry.mu.Unlock()

	existing, err := newRemoteTool(nil, &sdkmcp.Tool{
		Name:        "existing",
		InputSchema: map[string]any{"type": "object"},
	}, "remote", time.Second)
	if err != nil {
		t.Fatalf("existing tool: %v", err)
	}
	if err := registry.install("remote", 0, nil, nil, []*remoteTool{existing}, existing.lifecycle); err != nil {
		t.Fatalf("install existing tool: %v", err)
	}

	first, err := newRemoteTool(nil, &sdkmcp.Tool{
		Name:        "foo.bar",
		InputSchema: map[string]any{"type": "object"},
	}, "remote", time.Second)
	if err != nil {
		t.Fatalf("first tool: %v", err)
	}
	second, err := newRemoteTool(nil, &sdkmcp.Tool{
		Name:        "foo_bar",
		InputSchema: map[string]any{"type": "object"},
	}, "remote", time.Second)
	if err != nil {
		t.Fatalf("second tool: %v", err)
	}
	if err := registry.install("remote", 0, nil, nil, []*remoteTool{first, second}, first.lifecycle); err == nil {
		t.Fatal("install accepted a normalized collision")
	}
	if _, ok := registry.tools.Get(existing.exposedName); !ok {
		t.Fatal("collision removed the previously installed tool")
	}
	if server, ok := registry.Get(t.Context(), "remote"); !ok || server.ToolCount != 1 {
		t.Fatalf("server after collision = %#v, ok=%v", server, ok)
	}
}

func TestRegistryIgnoresNonCanonicalConfigSuffix(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mcp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	config, err := json.Marshal(domainmcp.Config{
		Type:    domainmcp.TransportHTTP,
		Enabled: false,
		URL:     "https://example.com/mcp",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "remote.JSON"), config, 0o644); err != nil {
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
	if servers := registry.List(t.Context()); len(servers) != 0 {
		t.Fatalf("servers = %#v, want no servers", servers)
	}
}

func TestMCPHTTPClientRejectsCrossOriginRedirect(t *testing.T) {
	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		targetRequests.Add(1)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL, http.StatusFound)
	}))
	defer origin.Close()
	endpoint, err := url.Parse(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := newMCPHTTPClient(endpoint, map[string]string{
		"Authorization": "Bearer secret",
		"X-Token":       "secret",
	})
	if _, err := client.Get(origin.URL); err == nil {
		t.Fatal("cross-origin redirect was accepted")
	}
	if targetRequests.Load() != 0 {
		t.Fatalf("cross-origin redirect reached target %d times", targetRequests.Load())
	}
}

func TestMCPHTTPClientAllowsSameOriginRedirectWithHeaders(t *testing.T) {
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/start" {
			http.Redirect(writer, request, "/done", http.StatusFound)
			return
		}
		received <- request.Header.Get("X-Token")
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := newMCPHTTPClient(endpoint, map[string]string{"X-Token": "secret"})
	response, err := client.Get(server.URL + "/start")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	response.Body.Close()
	if got := <-received; got != "secret" {
		t.Fatalf("same-origin header = %q, want secret", got)
	}
}

func TestRegistryRefreshKeepsToolSnapshotActive(t *testing.T) {
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

	registry.mu.Lock()
	registry.servers["remote"] = &serverEntry{
		name: "remote", config: domainmcp.Config{Type: domainmcp.TransportHTTP, Enabled: true, URL: "https://example.com/mcp"},
		tools: make(map[string]*remoteTool),
	}
	registry.mu.Unlock()

	first, err := newRemoteTool(nil, &sdkmcp.Tool{Name: "search", InputSchema: map[string]any{"type": "object"}}, "remote", time.Second)
	if err != nil {
		t.Fatalf("first tool: %v", err)
	}
	if err := registry.install("remote", 0, nil, nil, []*remoteTool{first}, first.lifecycle); err != nil {
		t.Fatalf("first install: %v", err)
	}
	second, err := newRemoteToolWithLifecycle(nil, &sdkmcp.Tool{Name: "search", InputSchema: map[string]any{"type": "object"}}, "remote", time.Second, first.lifecycle)
	if err != nil {
		t.Fatalf("second tool: %v", err)
	}
	if err := registry.install("remote", 0, nil, nil, []*remoteTool{second}, first.lifecycle); err != nil {
		t.Fatalf("refresh install: %v", err)
	}
	if !first.lifecycle.active.Load() {
		t.Fatal("old tool snapshot became inactive after refresh")
	}
}

func TestOAuthRefreshPersistsUpdatedToken(t *testing.T) {
	t.Parallel()

	var refreshes atomic.Int32
	tokenServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/token" {
			http.NotFound(writer, request)
			return
		}
		refreshes.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"access_token":"refreshed","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh-2"}`)
	}))
	defer tokenServer.Close()

	registry, err := NewRegistry(context.Background(), filepath.Join(t.TempDir(), "mcp"), agentloop.NewRegistry(), Options{})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	defer func() {
		if err := registry.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	}()
	serverConfig := domainmcp.Config{
		Type:    domainmcp.TransportHTTP,
		Enabled: false,
		URL:     "https://example.com/mcp",
	}
	if _, err := registry.Create(t.Context(), "remote", serverConfig); err != nil {
		t.Fatalf("Create: %v", err)
	}

	oauthConfig := &oauth2.Config{
		ClientID: "client",
		Endpoint: oauth2.Endpoint{TokenURL: tokenServer.URL + "/token"},
	}
	initial := &oauth2.Token{
		AccessToken:  "expired",
		RefreshToken: "refresh-1",
		Expiry:       time.Now().Add(-time.Minute),
	}
	refreshContext := context.WithValue(context.Background(), oauth2.HTTPClient, tokenServer.Client())
	source := registry.savingTokenSource("remote", 0, oauthConfig, initial, oauthConfig.TokenSource(refreshContext, initial))
	token, err := source.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if token.AccessToken != "refreshed" || refreshes.Load() != 1 {
		t.Fatalf("refreshed token = %#v, refreshes = %d", token, refreshes.Load())
	}

	stored, err := registry.loadOAuthSession("remote", serverConfig)
	if err != nil {
		t.Fatalf("loadOAuthSession: %v", err)
	}
	if stored.Config == nil || stored.Config.ClientID != "client" || stored.Token.AccessToken != "refreshed" || stored.Token.RefreshToken != "refresh-2" {
		t.Fatalf("stored OAuth session = %#v", stored)
	}
	if stored.TargetFingerprint == "" {
		t.Fatal("stored OAuth session has no target fingerprint")
	}
}

func TestOAuthSessionTargetBinding(t *testing.T) {
	registry, err := NewRegistry(context.Background(), filepath.Join(t.TempDir(), "mcp"), agentloop.NewRegistry(), Options{})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	defer func() {
		if err := registry.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	}()
	config := domainmcp.Config{
		Type:    domainmcp.TransportHTTP,
		Enabled: false,
		URL:     "https://example.com/mcp",
		Headers: map[string]string{"X-Region": "us-east-1"},
	}
	if _, err := registry.Create(t.Context(), "remote", config); err != nil {
		t.Fatalf("Create: %v", err)
	}
	oauthConfig := &oauth2.Config{ClientID: "client"}
	if err := registry.persistOAuthSession("remote", 0, oauthConfig, &oauth2.Token{AccessToken: "access"}); err != nil {
		t.Fatalf("persistOAuthSession: %v", err)
	}
	if _, err := registry.loadOAuthSession("remote", config); err != nil {
		t.Fatalf("loadOAuthSession with matching target: %v", err)
	}
	changed := config
	changed.URL = "https://replacement.example.com/mcp"
	if _, err := registry.loadOAuthSession("remote", changed); err == nil {
		t.Fatal("loadOAuthSession accepted a credential for a different target")
	}
}

func TestStaleOAuthRefreshCannotRestoreCredentials(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*Registry) error
	}{
		{
			name: "logout",
			mutate: func(registry *Registry) error {
				_, err := registry.Logout(context.Background(), "remote")
				return err
			},
		},
		{
			name: "remove",
			mutate: func(registry *Registry) error {
				return registry.Remove(context.Background(), "remote")
			},
		},
		{
			name: "update-target",
			mutate: func(registry *Registry) error {
				_, err := registry.Update(context.Background(), "remote", domainmcp.Config{
					Type:    domainmcp.TransportHTTP,
					Enabled: false,
					URL:     "https://replacement.example.com/mcp",
				})
				return err
			},
		},
	}

	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			registry, err := NewRegistry(context.Background(), filepath.Join(t.TempDir(), "mcp"), agentloop.NewRegistry(), Options{})
			if err != nil {
				t.Fatalf("NewRegistry: %v", err)
			}
			defer func() {
				if err := registry.Shutdown(context.Background()); err != nil {
					t.Errorf("Shutdown: %v", err)
				}
			}()
			if _, err := registry.Create(t.Context(), "remote", domainmcp.Config{
				Type:    domainmcp.TransportHTTP,
				Enabled: false,
				URL:     "https://example.com/mcp",
			}); err != nil {
				t.Fatalf("Create: %v", err)
			}

			started := make(chan struct{})
			release := make(chan struct{})
			source := registry.savingTokenSource(
				"remote",
				0,
				&oauth2.Config{ClientID: "client"},
				&oauth2.Token{AccessToken: "old"},
				&blockingTokenSource{started: started, release: release, token: &oauth2.Token{AccessToken: "refreshed"}},
			)
			result := make(chan error, 1)
			go func() {
				_, err := source.Token()
				result <- err
			}()
			<-started
			if err := mutation.mutate(registry); err != nil {
				t.Fatalf("%s: %v", mutation.name, err)
			}
			close(release)
			if err := <-result; err == nil {
				t.Fatal("stale refresh unexpectedly succeeded")
			}
			if _, err := os.Stat(registry.tokenPath("remote")); !os.IsNotExist(err) {
				t.Fatalf("stale refresh restored credentials, stat err = %v", err)
			}
		})
	}
}

type blockingTokenSource struct {
	started chan struct{}
	release <-chan struct{}
	token   *oauth2.Token
	once    sync.Once
}

func (source *blockingTokenSource) Token() (*oauth2.Token, error) {
	source.once.Do(func() { close(source.started) })
	<-source.release
	return source.token, nil
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
