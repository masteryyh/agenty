package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	domainmcp "github.com/masteryyh/agenty-core/pkg/domain/mcp"
)

const (
	defaultConnectTimeout = 30 * time.Second
	defaultToolTimeout    = 2 * time.Minute
	defaultConcurrency    = 4
	loginTimeout          = 5 * time.Minute
	maxServerLogEntries   = 50
)

var errRegistryClosed = errors.New("mcp: registry is shutting down")

type RegistryErrorKind string

const (
	RegistryErrorValidation    RegistryErrorKind = "validation"
	RegistryErrorNotFound      RegistryErrorKind = "not-found"
	RegistryErrorAlreadyExists RegistryErrorKind = "already-exists"
)

type RegistryError struct {
	Kind RegistryErrorKind
	Err  error
}

func (err *RegistryError) Error() string {
	if err == nil || err.Err == nil {
		return "MCP registry error"
	}
	return err.Err.Error()
}

func (err *RegistryError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

func registryValidation(err error) error {
	if err == nil {
		return nil
	}
	return &RegistryError{Kind: RegistryErrorValidation, Err: err}
}

func registryNotFound(err error) error {
	return &RegistryError{Kind: RegistryErrorNotFound, Err: err}
}

func registryAlreadyExists(err error) error {
	return &RegistryError{Kind: RegistryErrorAlreadyExists, Err: err}
}

type EventHandler func(context.Context, domainmcp.Event)

type Options struct {
	Logger             *slog.Logger
	Events             EventHandler
	ConnectTimeout     time.Duration
	ToolTimeout        time.Duration
	ConnectConcurrency int
}

type Registry struct {
	ctx               context.Context
	cancel            context.CancelFunc
	sessionBase       context.Context
	sessionBaseCancel context.CancelFunc
	dir               string
	authDir           string
	tools             *agentloop.Registry
	logger            *slog.Logger
	events            EventHandler
	connectWait       time.Duration
	toolWait          time.Duration
	semaphore         chan struct{}

	mu        sync.RWMutex
	servers   map[string]*serverEntry
	closing   bool
	startOnce sync.Once
	closeOnce sync.Once
	wait      sync.WaitGroup
}

var _ io.Closer = (*Registry)(nil)

type serverEntry struct {
	name          string
	config        domainmcp.Config
	status        domainmcp.Status
	err           string
	errorStage    string
	toolCount     int
	lastChange    time.Time
	deprecated    bool
	authURL       string
	logs          []domainmcp.LogEntry
	generation    uint64
	session       *mcp.ClientSession
	sessionCancel context.CancelFunc
	tools         map[string]*remoteTool
	loginCancel   context.CancelFunc
}

type tokenFile struct {
	Token oauth2.Token `json:"token"`
}

func NewRegistry(parent context.Context, dir string, tools *agentloop.Registry, options Options) (*Registry, error) {
	if parent == nil {
		return nil, fmt.Errorf("mcp: parent context must not be nil")
	}
	if tools == nil {
		return nil, fmt.Errorf("mcp: tool registry must not be nil")
	}
	if dir == "" {
		return nil, fmt.Errorf("mcp: configuration directory must not be empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("mcp: create configuration directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("mcp: secure configuration directory: %w", err)
	}
	authDir := filepath.Join(filepath.Dir(dir), "mcp-auth")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		return nil, fmt.Errorf("mcp: create auth directory: %w", err)
	}
	if err := os.Chmod(authDir, 0o700); err != nil {
		return nil, fmt.Errorf("mcp: secure auth directory: %w", err)
	}
	ctx, cancel := context.WithCancel(parent)
	// Work scheduled by the registry follows the parent context, while an
	// established MCP session gets an independent base context. This lets
	// Shutdown perform the protocol close before a signal or parent cancellation
	// tears down a stdio child process.
	sessionBase, sessionBaseCancel := context.WithCancel(context.WithoutCancel(parent))
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	connectTimeout := options.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = defaultConnectTimeout
	}
	toolTimeout := options.ToolTimeout
	if toolTimeout <= 0 {
		toolTimeout = defaultToolTimeout
	}
	concurrency := options.ConnectConcurrency
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}
	registry := &Registry{
		ctx:               ctx,
		cancel:            cancel,
		sessionBase:       sessionBase,
		sessionBaseCancel: sessionBaseCancel,
		dir:               dir,
		authDir:           authDir,
		tools:             tools,
		logger:            logger,
		events:            options.Events,
		connectWait:       connectTimeout,
		toolWait:          toolTimeout,
		semaphore:         make(chan struct{}, concurrency),
		servers:           make(map[string]*serverEntry),
	}
	if err := registry.load(); err != nil {
		sessionBaseCancel()
		cancel()
		return nil, err
	}
	return registry, nil
}

// Start schedules all enabled servers and returns immediately. The caller can
// safely construct the RPC server and begin serving while connections progress.
func (registry *Registry) Start() {
	registry.startOnce.Do(func() {
		registry.mu.RLock()
		names := make([]string, 0, len(registry.servers))
		for name, entry := range registry.servers {
			if entry.config.Enabled {
				names = append(names, name)
			}
		}
		registry.mu.RUnlock()
		sort.Strings(names)
		for _, name := range names {
			registry.scheduleConnect(name, false)
		}
	})
}

func (registry *Registry) load() error {
	entries, err := os.ReadDir(registry.dir)
	if err != nil {
		return fmt.Errorf("mcp: read configuration directory: %w", err)
	}
	for _, item := range entries {
		if item.IsDir() || strings.ToLower(filepath.Ext(item.Name())) != ".json" {
			continue
		}
		name := strings.TrimSuffix(item.Name(), filepath.Ext(item.Name()))
		if err := domainmcp.ValidateName(name); err != nil {
			registry.logger.Warn("ignoring MCP configuration with invalid name", "file", item.Name(), "error", err)
			continue
		}
		path := filepath.Join(registry.dir, item.Name())
		if err := os.Chmod(path, 0o600); err != nil {
			registry.logger.Warn("failed to secure MCP configuration file", "file", item.Name(), "error", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			registry.addLoadError(name, domainmcp.Config{}, "config", err)
			continue
		}
		var config domainmcp.Config
		if err := json.Unmarshal(data, &config); err != nil {
			registry.addLoadError(name, config, "config", err)
			continue
		}
		if err := config.Validate(); err != nil {
			registry.addLoadError(name, config, "config", err)
			continue
		}
		registry.servers[name] = &serverEntry{
			name:       name,
			config:     config,
			status:     statusForConfig(config),
			lastChange: time.Now(),
			deprecated: config.Type == domainmcp.TransportSSE,
			tools:      make(map[string]*remoteTool),
		}
	}
	return nil
}

func (registry *Registry) addLoadError(name string, config domainmcp.Config, stage string, err error) {
	entry := &serverEntry{
		name:       name,
		config:     config,
		status:     domainmcp.StatusError,
		err:        err.Error(),
		errorStage: stage,
		lastChange: time.Now(),
		deprecated: config.Type == domainmcp.TransportSSE,
		tools:      make(map[string]*remoteTool),
	}
	registry.appendLogLocked(entry, entry.generation, "error", "client", stage, entry.err)
	registry.servers[name] = entry
}

func statusForConfig(config domainmcp.Config) domainmcp.Status {
	if !config.Enabled {
		return domainmcp.StatusDisabled
	}
	return domainmcp.StatusConnecting
}

func (registry *Registry) List(_ context.Context) []domainmcp.Server {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	servers := make([]domainmcp.Server, 0, len(registry.servers))
	for _, entry := range registry.servers {
		servers = append(servers, registry.publicServerLocked(entry))
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].Name < servers[j].Name })
	return servers
}

func (registry *Registry) Get(_ context.Context, name string) (domainmcp.Server, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	entry, ok := registry.servers[name]
	if !ok {
		return domainmcp.Server{}, false
	}
	return registry.publicServerLocked(entry), true
}

func (registry *Registry) Logs(_ context.Context, name string) ([]domainmcp.LogEntry, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	entry, ok := registry.servers[name]
	if !ok {
		return nil, false
	}
	logs := make([]domainmcp.LogEntry, len(entry.logs))
	copy(logs, entry.logs)
	return logs, true
}

func (registry *Registry) publicServerLocked(entry *serverEntry) domainmcp.Server {
	config := cloneConfig(entry.config)
	return domainmcp.Server{
		Name:        entry.name,
		Config:      config,
		Status:      entry.status,
		Error:       entry.err,
		ErrorStage:  entry.errorStage,
		ToolCount:   entry.toolCount,
		LastChanged: entry.lastChange.UTC().Format(time.RFC3339Nano),
		Deprecated:  entry.deprecated,
		AuthURL:     entry.authURL,
	}
}

func (registry *Registry) Create(ctx context.Context, name string, config domainmcp.Config) (domainmcp.Server, error) {
	if err := domainmcp.ValidateName(name); err != nil {
		return domainmcp.Server{}, registryValidation(err)
	}
	if err := config.Validate(); err != nil {
		return domainmcp.Server{}, registryValidation(err)
	}
	config = cloneConfig(config)
	registry.mu.Lock()
	if registry.closing {
		registry.mu.Unlock()
		return domainmcp.Server{}, errRegistryClosed
	}
	if _, exists := registry.servers[name]; exists {
		registry.mu.Unlock()
		return domainmcp.Server{}, registryAlreadyExists(fmt.Errorf("mcp: server %q already exists", name))
	}
	if err := registry.persistConfig(name, config); err != nil {
		registry.mu.Unlock()
		return domainmcp.Server{}, err
	}
	entry := &serverEntry{
		name: name, config: config, status: statusForConfig(config), lastChange: time.Now(),
		deprecated: config.Type == domainmcp.TransportSSE, tools: make(map[string]*remoteTool),
	}
	registry.servers[name] = entry
	server := registry.publicServerLocked(entry)
	registry.mu.Unlock()
	registry.emit(domainmcp.Event{Type: "server_added", Name: name, Status: server.Status})
	if config.Enabled {
		registry.scheduleConnect(name, false)
	}
	return server, nil
}

func (registry *Registry) Update(ctx context.Context, name string, config domainmcp.Config) (domainmcp.Server, error) {
	if err := domainmcp.ValidateName(name); err != nil {
		return domainmcp.Server{}, registryValidation(err)
	}
	if err := config.Validate(); err != nil {
		return domainmcp.Server{}, registryValidation(err)
	}
	config = cloneConfig(config)
	registry.mu.Lock()
	if registry.closing {
		registry.mu.Unlock()
		return domainmcp.Server{}, errRegistryClosed
	}
	entry, ok := registry.servers[name]
	if !ok {
		registry.mu.Unlock()
		return domainmcp.Server{}, registryNotFound(fmt.Errorf("mcp: server %q not found", name))
	}
	if !sameOAuthTarget(entry.config, config) {
		if err := os.Remove(registry.tokenPath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			registry.mu.Unlock()
			return domainmcp.Server{}, fmt.Errorf("mcp: clear OAuth credentials: %w", err)
		}
	}
	if err := registry.persistConfig(name, config); err != nil {
		registry.mu.Unlock()
		return domainmcp.Server{}, err
	}
	session, sessionCancel := registry.invalidateLocked(entry)
	entry.config = config
	entry.deprecated = config.Type == domainmcp.TransportSSE
	entry.generation++
	entry.status = statusForConfig(config)
	entry.err = ""
	entry.errorStage = ""
	entry.authURL = ""
	entry.lastChange = time.Now()
	server := registry.publicServerLocked(entry)
	generation := entry.generation
	registry.mu.Unlock()
	registry.closeSession(session, sessionCancel)
	registry.emit(domainmcp.Event{Type: "server_updated", Name: name, Status: server.Status})
	if config.Enabled {
		registry.scheduleConnectAt(name, generation, false)
	}
	return server, nil
}

func (registry *Registry) SetEnabled(ctx context.Context, name string, enabled bool) (domainmcp.Server, error) {
	registry.mu.Lock()
	if registry.closing {
		registry.mu.Unlock()
		return domainmcp.Server{}, errRegistryClosed
	}
	entry, ok := registry.servers[name]
	if !ok {
		registry.mu.Unlock()
		return domainmcp.Server{}, registryNotFound(fmt.Errorf("mcp: server %q not found", name))
	}
	config := entry.config
	config.Enabled = enabled
	if err := config.Validate(); err != nil {
		registry.mu.Unlock()
		return domainmcp.Server{}, registryValidation(err)
	}
	if err := registry.persistConfig(name, config); err != nil {
		registry.mu.Unlock()
		return domainmcp.Server{}, err
	}
	session, sessionCancel := registry.invalidateLocked(entry)
	entry.config = config
	entry.generation++
	entry.status = statusForConfig(config)
	entry.err = ""
	entry.errorStage = ""
	entry.authURL = ""
	entry.lastChange = time.Now()
	server := registry.publicServerLocked(entry)
	generation := entry.generation
	registry.mu.Unlock()
	registry.closeSession(session, sessionCancel)
	registry.emit(domainmcp.Event{Type: "server_enabled_changed", Name: name, Status: server.Status})
	if config.Enabled {
		registry.scheduleConnectAt(name, generation, false)
	}
	return server, nil
}

func (registry *Registry) Remove(ctx context.Context, name string) error {
	if err := domainmcp.ValidateName(name); err != nil {
		return registryValidation(err)
	}
	registry.mu.Lock()
	if registry.closing {
		registry.mu.Unlock()
		return errRegistryClosed
	}
	entry, ok := registry.servers[name]
	if !ok {
		registry.mu.Unlock()
		return registryNotFound(fmt.Errorf("mcp: server %q not found", name))
	}
	if err := os.Remove(registry.tokenPath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		registry.mu.Unlock()
		return fmt.Errorf("mcp: remove OAuth credentials: %w", err)
	}
	if err := os.Remove(filepath.Join(registry.dir, name+".json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		registry.mu.Unlock()
		return fmt.Errorf("mcp: remove configuration: %w", err)
	}
	session, sessionCancel := registry.invalidateLocked(entry)
	delete(registry.servers, name)
	registry.mu.Unlock()
	registry.closeSession(session, sessionCancel)
	registry.emit(domainmcp.Event{Type: "server_removed", Name: name})
	return nil
}

func (registry *Registry) Reconnect(_ context.Context, name string) (domainmcp.Server, error) {
	registry.mu.Lock()
	if registry.closing {
		registry.mu.Unlock()
		return domainmcp.Server{}, errRegistryClosed
	}
	entry, ok := registry.servers[name]
	if !ok {
		registry.mu.Unlock()
		return domainmcp.Server{}, registryNotFound(fmt.Errorf("mcp: server %q not found", name))
	}
	if !entry.config.Enabled {
		server := registry.publicServerLocked(entry)
		registry.mu.Unlock()
		return server, registryValidation(fmt.Errorf("mcp: server %q is disabled", name))
	}
	session, sessionCancel := registry.invalidateLocked(entry)
	entry.generation++
	entry.status = domainmcp.StatusConnecting
	entry.err = ""
	entry.errorStage = ""
	entry.authURL = ""
	entry.lastChange = time.Now()
	generation := entry.generation
	server := registry.publicServerLocked(entry)
	registry.mu.Unlock()
	registry.closeSession(session, sessionCancel)
	registry.emit(domainmcp.Event{Type: "server_reconnecting", Name: name, Status: server.Status})
	registry.scheduleConnectAt(name, generation, false)
	return server, nil
}

func (registry *Registry) Login(_ context.Context, name string) (domainmcp.Server, error) {
	registry.mu.Lock()
	if registry.closing {
		registry.mu.Unlock()
		return domainmcp.Server{}, errRegistryClosed
	}
	entry, ok := registry.servers[name]
	if !ok {
		registry.mu.Unlock()
		return domainmcp.Server{}, registryNotFound(fmt.Errorf("mcp: server %q not found", name))
	}
	if !entry.config.Enabled {
		registry.mu.Unlock()
		return domainmcp.Server{}, registryValidation(fmt.Errorf("mcp: server %q is disabled", name))
	}
	if entry.status == domainmcp.StatusAuthenticating {
		registry.mu.Unlock()
		return domainmcp.Server{}, registryValidation(fmt.Errorf("mcp: OAuth login for server %q is already in progress", name))
	}
	if entry.config.Type == domainmcp.TransportStdio {
		registry.mu.Unlock()
		return domainmcp.Server{}, registryValidation(fmt.Errorf("mcp: stdio servers do not support OAuth login"))
	}
	if entry.config.Type == domainmcp.TransportSSE {
		registry.mu.Unlock()
		return domainmcp.Server{}, registryValidation(fmt.Errorf("mcp: OAuth login is not supported for deprecated SSE transport; use streamable HTTP"))
	}
	if err := os.Remove(registry.tokenPath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		registry.mu.Unlock()
		return domainmcp.Server{}, fmt.Errorf("mcp: clear existing OAuth credentials: %w", err)
	}
	session, sessionCancel := registry.invalidateLocked(entry)
	entry.generation++
	entry.status = domainmcp.StatusAuthenticating
	entry.err = ""
	entry.errorStage = ""
	entry.authURL = ""
	entry.lastChange = time.Now()
	generation := entry.generation
	server := registry.publicServerLocked(entry)
	loginTask := registry.startTaskLocked()
	registry.mu.Unlock()
	if !loginTask {
		return domainmcp.Server{}, errRegistryClosed
	}
	registry.closeSession(session, sessionCancel)
	registry.emit(domainmcp.Event{Type: "auth_started", Name: name, Status: server.Status})
	go func() {
		defer registry.wait.Done()
		select {
		case registry.semaphore <- struct{}{}:
		case <-registry.ctx.Done():
			return
		}
		defer func() { <-registry.semaphore }()
		registry.login(name, generation)
	}()
	return server, nil
}

func (registry *Registry) Logout(_ context.Context, name string) (domainmcp.Server, error) {
	registry.mu.Lock()
	if registry.closing {
		registry.mu.Unlock()
		return domainmcp.Server{}, errRegistryClosed
	}
	entry, ok := registry.servers[name]
	if !ok {
		registry.mu.Unlock()
		return domainmcp.Server{}, registryNotFound(fmt.Errorf("mcp: server %q not found", name))
	}
	if err := os.Remove(registry.tokenPath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		registry.mu.Unlock()
		return domainmcp.Server{}, fmt.Errorf("mcp: remove OAuth credentials: %w", err)
	}
	session, sessionCancel := registry.invalidateLocked(entry)
	enabled := entry.config.Enabled
	entry.generation++
	entry.status = statusForConfig(entry.config)
	entry.err = ""
	entry.errorStage = ""
	entry.authURL = ""
	entry.lastChange = time.Now()
	generation := entry.generation
	server := registry.publicServerLocked(entry)
	registry.mu.Unlock()
	registry.closeSession(session, sessionCancel)
	registry.emit(domainmcp.Event{Type: "auth_logged_out", Name: name, Status: server.Status})
	if enabled {
		registry.scheduleConnectAt(name, generation, false)
	}
	return server, nil
}

func (registry *Registry) scheduleConnect(name string, interactive bool) {
	registry.mu.RLock()
	entry, ok := registry.servers[name]
	var generation uint64
	if ok {
		generation = entry.generation
	}
	registry.mu.RUnlock()
	if ok {
		registry.scheduleConnectAt(name, generation, interactive)
	}
}

func (registry *Registry) scheduleConnectAt(name string, generation uint64, interactive bool) {
	if !registry.startTask() {
		return
	}
	go func() {
		defer registry.wait.Done()
		select {
		case registry.semaphore <- struct{}{}:
		case <-registry.ctx.Done():
			return
		}
		defer func() { <-registry.semaphore }()
		registry.connect(name, generation, interactive)
	}()
}

func (registry *Registry) newClient(name string, generation uint64, transportType domainmcp.Transport) *mcp.Client {
	options := &mcp.ClientOptions{Logger: registry.logger}
	// Some Streamable HTTP servers reject subscriptions/listen even when
	// server/discover and tools/list are supported.
	if transportType != domainmcp.TransportHTTP {
		options.ToolListChangedHandler = func(context.Context, *mcp.ToolListChangedRequest) {
			registry.refreshAsync(name, generation)
		}
	}
	return mcp.NewClient(&mcp.Implementation{Name: "agenty", Version: "0.1.0"}, options)
}

func (registry *Registry) connect(name string, generation uint64, interactive bool) {
	registry.mu.RLock()
	entry, ok := registry.servers[name]
	if !ok || entry.generation != generation || !entry.config.Enabled || registry.closing || registry.ctx.Err() != nil {
		registry.mu.RUnlock()
		return
	}
	config := entry.config
	registry.mu.RUnlock()
	registry.appendServerLog(name, generation, "info", "client", "connect", "connecting")

	// The context passed to mcp.Client.Connect becomes the lifetime context of
	// the returned session. Use a manually-cancelled context and a timer for the
	// handshake deadline so a successful session is not torn down when the
	// connect function returns.
	sessionCtx, sessionCancel := context.WithCancel(registry.sessionBase)
	transport, oauthHandler, cleanup, err := registry.transport(sessionCtx, name, config, interactive, generation)
	if err != nil {
		sessionCancel()
		registry.setError(name, generation, "transport", err, interactive)
		return
	}
	defer cleanup()

	client := registry.newClient(name, generation, config.Type)
	if oauthHandler != nil {
		switch value := transport.(type) {
		case *mcp.StreamableClientTransport:
			value.OAuthHandler = oauthHandler
		}
	}
	connectTimer := time.AfterFunc(registry.connectWait, sessionCancel)
	session, err := client.Connect(sessionCtx, transport, nil)
	connectTimer.Stop()
	if err == nil && sessionCtx.Err() != nil {
		// The handshake deadline or registry shutdown raced with Connect. Treat
		// a session returned after cancellation as unusable.
		err = sessionCtx.Err()
	}
	if err != nil {
		closeMCPConnection(session, sessionCancel, registry.logger)
		if requiresAuth(err) {
			registry.setAuthRequired(name, generation, err)
		} else {
			registry.setError(name, generation, "connect", err, interactive)
		}
		return
	}
	toolsCtx, toolsCancel := context.WithTimeout(sessionCtx, registry.connectWait)
	tools, err := listTools(toolsCtx, session, name, registry.toolWait)
	toolsCancel()
	if err != nil {
		closeMCPConnection(session, sessionCancel, registry.logger)
		registry.setError(name, generation, "tools/list", err, interactive)
		return
	}
	if err := registry.install(name, generation, session, sessionCancel, tools); err != nil {
		closeMCPConnection(session, sessionCancel, registry.logger)
		registry.setError(name, generation, "tool registration", err, interactive)
		return
	}
	if oauthHandler != nil {
		if err := registry.persistOAuthToken(name, oauthHandler); err != nil {
			registry.logger.Warn("failed to persist MCP OAuth credentials", "server", name, "error", err)
		}
	}
}

func (registry *Registry) transport(ctx context.Context, name string, config domainmcp.Config, interactive bool, generation uint64) (mcp.Transport, auth.OAuthHandler, func(), error) {
	cleanup := func() {}
	if err := config.Validate(); err != nil {
		return nil, nil, cleanup, err
	}
	headers := expandMap(config.Headers)
	var oauthHandler auth.OAuthHandler
	var err error
	if config.BearerTokenEnvVar != "" {
		if token := os.Getenv(config.BearerTokenEnvVar); token != "" {
			headers["Authorization"] = "Bearer " + token
		}
	}
	if config.Type == domainmcp.TransportHTTP {
		if token, err := registry.loadOAuthToken(name); err == nil && token.AccessToken != "" {
			headers["Authorization"] = bearerValue(token)
		}
	}
	httpClient := &http.Client{Transport: &headerRoundTripper{base: http.DefaultTransport, headers: headers}}
	switch config.Type {
	case domainmcp.TransportStdio:
		// The session context is retained for the lifetime of the MCP session.
		// CommandTransport performs the protocol-aware stdio shutdown; the
		// command context is a fallback for a failed handshake or forced close.
		command := exec.CommandContext(ctx, config.Command, config.Args...)
		command.Env = append(os.Environ(), envList(expandMap(config.Env))...)
		command.Stderr = &stdioLogWriter{registry: registry, name: name, generation: generation}
		return &mcp.CommandTransport{Command: command}, nil, cleanup, nil
	case domainmcp.TransportHTTP:
		if interactive {
			var handlerCleanup func()
			// MCP request headers can contain bearer credentials. Keep them out of
			// OAuth discovery, registration, and token requests to avoid sending a
			// resource-server secret to a different authorization host.
			oauthClient := &http.Client{Transport: http.DefaultTransport, Timeout: registry.connectWait}
			oauthHandler, handlerCleanup, err = registry.newOAuthHandler(name, config, oauthClient, generation)
			if err != nil {
				return nil, nil, cleanup, err
			}
			cleanup = handlerCleanup
		}
		return &mcp.StreamableClientTransport{Endpoint: config.URL, HTTPClient: httpClient}, oauthHandler, cleanup, nil
	case domainmcp.TransportSSE:
		return &mcp.SSEClientTransport{Endpoint: config.URL, HTTPClient: httpClient}, nil, cleanup, nil
	default:
		return nil, nil, cleanup, fmt.Errorf("mcp: unsupported transport %q", config.Type)
	}
}

func (registry *Registry) install(name string, generation uint64, session *mcp.ClientSession, sessionCancel context.CancelFunc, tools []*remoteTool) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	entry, ok := registry.servers[name]
	if !ok || entry.generation != generation || !entry.config.Enabled || registry.closing {
		return fmt.Errorf("MCP server %q changed while connecting", name)
	}
	newSession := entry.session != session
	oldNames := make(map[string]struct{}, len(entry.tools))
	for exposed := range entry.tools {
		oldNames[exposed] = struct{}{}
	}
	for _, tool := range tools {
		if _, exists := registry.tools.Get(tool.exposedName); exists {
			if _, isOld := oldNames[tool.exposedName]; !isOld {
				return fmt.Errorf("tool name %q conflicts with an existing tool", tool.exposedName)
			}
		}
	}
	for exposed, tool := range entry.tools {
		tool.active.Store(false)
		registry.tools.Unregister(exposed)
	}
	for _, tool := range tools {
		if err := registry.tools.Register(tool); err != nil {
			for _, registered := range tools {
				registered.active.Store(false)
				registry.tools.Unregister(registered.exposedName)
			}
			return err
		}
		tool.active.Store(true)
	}
	entry.session = session
	entry.sessionCancel = sessionCancel
	// Once a session is installed, its lifetime is managed by sessionCancel;
	// loginCancel is only needed while the interactive authorization handshake
	// is waiting for the browser callback.
	entry.loginCancel = nil
	entry.tools = make(map[string]*remoteTool, len(tools))
	for _, tool := range tools {
		entry.tools[tool.exposedName] = tool
	}
	entry.toolCount = len(tools)
	entry.status = domainmcp.StatusConnected
	entry.err = ""
	entry.errorStage = ""
	entry.authURL = ""
	entry.lastChange = time.Now()
	registry.appendLogLocked(entry, generation, "info", "client", "connected", fmt.Sprintf("connected with %d tools", len(tools)))
	registry.emitLocked(domainmcp.Event{Type: "server_connected", Name: name, Status: entry.status, ToolCount: len(tools)})
	if newSession && registry.startTaskLocked() {
		go func() {
			defer registry.wait.Done()
			registry.monitorSession(name, generation, session)
		}()
	}
	return nil
}

func (registry *Registry) monitorSession(name string, generation uint64, session *mcp.ClientSession) {
	err := session.Wait()
	registry.mu.RLock()
	entry, ok := registry.servers[name]
	current := ok && entry.generation == generation && entry.session == session && !registry.closing
	registry.mu.RUnlock()
	if !current {
		return
	}
	if err == nil {
		err = errors.New("MCP session closed by server")
	}
	registry.setError(name, generation, "connection", err, false)
}

func (registry *Registry) refreshAsync(name string, generation uint64) {
	if !registry.startTask() {
		return
	}
	go func() {
		defer registry.wait.Done()
		registry.refresh(name, generation)
	}()
}

func (registry *Registry) refresh(name string, generation uint64) {
	registry.mu.RLock()
	entry, ok := registry.servers[name]
	if !ok || entry.generation != generation || entry.session == nil || registry.closing {
		registry.mu.RUnlock()
		return
	}
	session := entry.session
	sessionCancel := entry.sessionCancel
	registry.mu.RUnlock()
	ctx, cancel := context.WithTimeout(registry.ctx, registry.connectWait)
	defer cancel()
	tools, err := listTools(ctx, session, name, registry.toolWait)
	if err != nil {
		registry.setError(name, generation, "tools/list", err, false)
		return
	}
	if err := registry.install(name, generation, session, sessionCancel, tools); err != nil {
		registry.setError(name, generation, "tool registration", err, false)
	}
}

func (registry *Registry) setAuthRequired(name string, generation uint64, err error) {
	registry.mu.Lock()
	entry, ok := registry.servers[name]
	if !ok || entry.generation != generation || registry.closing {
		registry.mu.Unlock()
		return
	}
	session, sessionCancel := registry.invalidateLocked(entry)
	entry.status = domainmcp.StatusAuthRequired
	entry.err = err.Error()
	entry.errorStage = "authorize"
	entry.lastChange = time.Now()
	registry.appendLogLocked(entry, generation, "warn", "client", entry.errorStage, entry.err)
	registry.logger.Warn("MCP server requires authorization", "server", name, "error", err)
	event := domainmcp.Event{Type: "auth_required", Name: name, Status: entry.status, Error: entry.err, ErrorStage: entry.errorStage}
	registry.mu.Unlock()
	registry.closeSession(session, sessionCancel)
	registry.emit(event)
}

func (registry *Registry) setError(name string, generation uint64, stage string, err error, _ bool) {
	registry.mu.Lock()
	entry, ok := registry.servers[name]
	if !ok || entry.generation != generation || registry.closing {
		registry.mu.Unlock()
		return
	}
	session, sessionCancel := registry.invalidateLocked(entry)
	entry.status = domainmcp.StatusError
	if requiresAuth(err) {
		entry.status = domainmcp.StatusAuthRequired
	}
	entry.err = err.Error()
	entry.errorStage = stage
	entry.lastChange = time.Now()
	registry.appendLogLocked(entry, generation, "error", "client", stage, entry.err)
	registry.logger.Warn("MCP server connection failed", "server", name, "stage", stage, "error", err)
	event := domainmcp.Event{Type: "server_error", Name: name, Status: entry.status, Error: entry.err, ErrorStage: stage}
	registry.mu.Unlock()
	registry.closeSession(session, sessionCancel)
	registry.emit(event)
}

func (registry *Registry) invalidateLocked(entry *serverEntry) (*mcp.ClientSession, context.CancelFunc) {
	session := entry.session
	sessionCancel := entry.sessionCancel
	for exposed, tool := range entry.tools {
		tool.active.Store(false)
		registry.tools.Unregister(exposed)
	}
	entry.tools = make(map[string]*remoteTool)
	entry.toolCount = 0
	entry.session = nil
	entry.sessionCancel = nil
	if entry.loginCancel != nil {
		entry.loginCancel()
		entry.loginCancel = nil
	}
	return session, sessionCancel
}

func (registry *Registry) closeSession(session *mcp.ClientSession, sessionCancel context.CancelFunc) {
	if session == nil {
		if sessionCancel != nil {
			sessionCancel()
		}
		return
	}
	if !registry.startTask() {
		// A mutation can race with shutdown after it has detached its old
		// session from the registry. Close it inline so shutdown cannot leak
		// the subprocess or HTTP connection.
		closeMCPConnection(session, sessionCancel, registry.logger)
		return
	}
	go func() {
		defer registry.wait.Done()
		closeMCPConnection(session, sessionCancel, registry.logger)
	}()
}

func closeMCPConnection(session *mcp.ClientSession, sessionCancel context.CancelFunc, logger *slog.Logger) {
	if session != nil {
		if err := session.Close(); err != nil && !errors.Is(err, mcp.ErrConnectionClosed) {
			logger.Debug("MCP session close failed", "error", err)
		}
	}
	if sessionCancel != nil {
		sessionCancel()
	}
}

func (registry *Registry) startTask() bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return registry.startTaskLocked()
}

func (registry *Registry) startTaskLocked() bool {
	if registry.closing {
		return false
	}
	registry.wait.Add(1)
	return true
}

func (registry *Registry) emit(event domainmcp.Event) {
	if registry.events == nil {
		return
	}
	registry.mu.Lock()
	if !registry.startTaskLocked() {
		registry.mu.Unlock()
		return
	}
	registry.mu.Unlock()
	go func() {
		defer registry.wait.Done()
		registry.events(registry.ctx, event)
	}()
}

func (registry *Registry) emitLocked(event domainmcp.Event) {
	if registry.events == nil {
		return
	}
	if !registry.startTaskLocked() {
		return
	}
	go func() {
		defer registry.wait.Done()
		registry.events(registry.ctx, event)
	}()
}

func (registry *Registry) persistConfig(name string, config domainmcp.Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("mcp: encode configuration: %w", err)
	}
	path := filepath.Join(registry.dir, name+".json")
	tmp, err := os.CreateTemp(registry.dir, ".mcp-*.json")
	if err != nil {
		return fmt.Errorf("mcp: create configuration temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("mcp: set configuration permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("mcp: write configuration: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("mcp: sync configuration: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("mcp: close configuration: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("mcp: replace configuration: %w", err)
	}
	return nil
}

func cloneConfig(config domainmcp.Config) domainmcp.Config {
	clone := config
	clone.Args = append([]string(nil), config.Args...)
	clone.Env = cloneStringMap(config.Env)
	clone.Headers = cloneStringMap(config.Headers)
	if config.OAuth != nil {
		oauth := *config.OAuth
		clone.OAuth = &oauth
	}
	return clone
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func sameOAuthTarget(left, right domainmcp.Config) bool {
	if left.Type != right.Type || left.URL != right.URL || left.BearerTokenEnvVar != right.BearerTokenEnvVar {
		return false
	}
	if !sameStringMap(left.Headers, right.Headers) {
		return false
	}
	if left.OAuth == nil || right.OAuth == nil {
		return left.OAuth == nil && right.OAuth == nil
	}
	return *left.OAuth == *right.OAuth
}

func sameStringMap(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func listTools(ctx context.Context, session *mcp.ClientSession, serverName string, timeout time.Duration) ([]*remoteTool, error) {
	var result []*remoteTool
	params := &mcp.ListToolsParams{}
	for {
		page, err := session.ListTools(ctx, params)
		if err != nil {
			return nil, err
		}
		for _, definition := range page.Tools {
			if definition == nil {
				continue
			}
			tool, err := newRemoteTool(session, definition, serverName, timeout)
			if err != nil {
				return nil, err
			}
			result = append(result, tool)
		}
		if page.NextCursor == "" {
			return result, nil
		}
		params = &mcp.ListToolsParams{Cursor: page.NextCursor}
	}
}

type remoteTool struct {
	session     *mcp.ClientSession
	remoteName  string
	exposedName string
	definition  agentloop.ToolDefinition
	active      atomic.Bool
	timeout     time.Duration
}

func newRemoteTool(session *mcp.ClientSession, definition *mcp.Tool, serverName string, timeout time.Duration) (*remoteTool, error) {
	if definition.Name == "" || strings.ContainsAny(definition.Name, "\x00\r\n\t ") {
		return nil, fmt.Errorf("mcp: invalid tool name %q", definition.Name)
	}
	schemaData, err := json.Marshal(definition.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("mcp: encode input schema for %q: %w", definition.Name, err)
	}
	var schema agentloop.JSONSchema
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		return nil, fmt.Errorf("mcp: decode input schema for %q: %w", definition.Name, err)
	}
	if timeout <= 0 {
		timeout = defaultToolTimeout
	}
	exposedName := definition.Name
	if serverName != "" {
		exposedName = "mcp__" + serverName + "__" + definition.Name
	}
	return &remoteTool{
		session:     session,
		remoteName:  definition.Name,
		exposedName: exposedName,
		definition: agentloop.ToolDefinition{
			Type:        agentloop.ToolTypeFunction,
			Name:        exposedName,
			Description: definition.Description,
			InputSchema: schema,
		},
		timeout: timeout,
	}, nil
}

func (tool *remoteTool) Definition() agentloop.ToolDefinition {
	return tool.definition
}

func (tool *remoteTool) Execute(ctx context.Context, _ agentloop.CallContext, input []byte) (conversation.Content, error) {
	if !tool.active.Load() {
		return nil, fmt.Errorf("MCP tool %q is no longer available", tool.definition.Name)
	}
	var arguments any
	if len(input) == 0 {
		arguments = map[string]any{}
	} else if err := json.Unmarshal(input, &arguments); err != nil {
		return nil, fmt.Errorf("decode arguments for MCP tool %q: %w", tool.definition.Name, err)
	}
	callCtx, cancel := context.WithTimeout(ctx, tool.timeout)
	defer cancel()
	result, err := tool.session.CallTool(callCtx, &mcp.CallToolParams{Name: tool.remoteName, Arguments: arguments})
	if err != nil {
		return nil, fmt.Errorf("call MCP tool %q: %w", tool.definition.Name, err)
	}
	content, err := convertContent(result.Content, result.StructuredContent)
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return content, &agentloop.ToolExecutionError{Content: content, Err: fmt.Errorf("MCP tool %q returned an error", tool.definition.Name)}
	}
	return content, nil
}

func convertContent(values []mcp.Content, structured any) (conversation.Content, error) {
	content := make(conversation.Content, 0, len(values)+1)
	for _, value := range values {
		switch item := value.(type) {
		case *mcp.TextContent:
			content = append(content, conversation.TextBlock{Text: item.Text})
		case *mcp.ImageContent:
			content = append(content, conversation.ImageBlock{MimeType: item.MIMEType, Data: base64.StdEncoding.EncodeToString(item.Data)})
		default:
			data, err := json.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("encode MCP tool result: %w", err)
			}
			content = append(content, conversation.TextBlock{Text: string(data)})
		}
	}
	if structured != nil {
		data, err := json.Marshal(structured)
		if err != nil {
			return nil, fmt.Errorf("encode MCP structured result: %w", err)
		}
		content = append(content, conversation.TextBlock{Text: string(data)})
	}
	if len(content) == 0 {
		content = append(content, conversation.TextBlock{Text: ""})
	}
	return content, nil
}

func (registry *Registry) newOAuthHandler(name string, config domainmcp.Config, httpClient *http.Client, generation uint64) (auth.OAuthHandler, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, func() {}, fmt.Errorf("mcp: listen for OAuth callback: %w", err)
	}
	callbackURL := "http://" + listener.Addr().String() + "/oauth/callback"
	codeCh := make(chan *auth.AuthorizationResult, 1)
	callbackServer := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/oauth/callback" {
			http.NotFound(writer, request)
			return
		}
		result := &auth.AuthorizationResult{Code: request.URL.Query().Get("code"), State: request.URL.Query().Get("state"), Iss: request.URL.Query().Get("iss")}
		if request.URL.Query().Get("error") != "" {
			result.Code = ""
		}
		select {
		case codeCh <- result:
		default:
		}
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(writer, "Authorization received. You can return to agenty.\n")
	})}
	go func() {
		if err := callbackServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			registry.logger.Debug("OAuth callback server stopped", "server", name, "error", err)
		}
	}()

	metadata := &oauthex.ClientRegistrationMetadata{
		RedirectURIs:            []string{callbackURL},
		ClientName:              "Agenty",
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: "none",
	}
	options := &auth.AuthorizationCodeHandlerConfig{
		RedirectURL:                     callbackURL,
		RequestRefreshToken:             true,
		Client:                          httpClient,
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{Metadata: metadata},
		AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			registry.setAuthURL(name, generation, args.URL)
			select {
			case result := <-codeCh:
				if result.Code == "" {
					return nil, fmt.Errorf("OAuth authorization was cancelled")
				}
				return result, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}
	if config.OAuth != nil && config.OAuth.ClientID != "" {
		options.DynamicClientRegistrationConfig = nil
		options.PreregisteredClient = &oauthex.ClientCredentials{
			ClientID: os.ExpandEnv(config.OAuth.ClientID),
			Issuer:   os.ExpandEnv(config.OAuth.Issuer),
		}
		if config.OAuth.ClientSecret != "" {
			options.PreregisteredClient.ClientSecretAuth = &oauthex.ClientSecretAuth{ClientSecret: os.ExpandEnv(config.OAuth.ClientSecret)}
		}
	}
	handler, err := auth.NewAuthorizationCodeHandler(options)
	if err != nil {
		_ = callbackServer.Shutdown(context.Background())
		return nil, func() {}, err
	}
	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = callbackServer.Shutdown(shutdownCtx)
	}
	return handler, cleanup, nil
}

func (registry *Registry) login(name string, generation uint64) {
	registry.mu.RLock()
	entry, ok := registry.servers[name]
	if !ok || entry.generation != generation || registry.closing || registry.ctx.Err() != nil {
		registry.mu.RUnlock()
		return
	}
	config := entry.config
	registry.mu.RUnlock()
	sessionCtx, sessionCancel := context.WithCancel(registry.sessionBase)
	transport, handler, cleanup, err := registry.transport(sessionCtx, name, config, true, generation)
	if err != nil {
		sessionCancel()
		registry.setError(name, generation, "oauth", err, true)
		return
	}
	defer cleanup()
	if streamable, ok := transport.(*mcp.StreamableClientTransport); ok {
		streamable.OAuthHandler = handler
	}
	registry.mu.Lock()
	if current, exists := registry.servers[name]; exists && current.generation == generation {
		current.loginCancel = sessionCancel
	}
	registry.mu.Unlock()
	client := registry.newClient(name, generation, config.Type)
	loginTimer := time.AfterFunc(loginTimeout, sessionCancel)
	session, err := client.Connect(sessionCtx, transport, nil)
	loginTimer.Stop()
	if err == nil && sessionCtx.Err() != nil {
		err = sessionCtx.Err()
	}
	if err != nil {
		closeMCPConnection(session, sessionCancel, registry.logger)
		registry.setError(name, generation, "oauth", err, true)
		return
	}
	toolsCtx, toolsCancel := context.WithTimeout(sessionCtx, registry.connectWait)
	tools, err := listTools(toolsCtx, session, name, registry.toolWait)
	toolsCancel()
	if err != nil {
		closeMCPConnection(session, sessionCancel, registry.logger)
		registry.setError(name, generation, "tools/list", err, true)
		return
	}
	if err := registry.install(name, generation, session, sessionCancel, tools); err != nil {
		closeMCPConnection(session, sessionCancel, registry.logger)
		registry.setError(name, generation, "tool registration", err, true)
		return
	}
	registry.mu.Lock()
	if current, exists := registry.servers[name]; exists && current.generation == generation {
		current.loginCancel = nil
	}
	registry.mu.Unlock()
	registry.mu.RLock()
	current := false
	if entry, exists := registry.servers[name]; exists {
		current = entry.generation == generation && entry.session == session && !registry.closing
	}
	registry.mu.RUnlock()
	if current {
		if err := registry.persistOAuthToken(name, handler); err != nil {
			registry.logger.Warn("failed to persist MCP OAuth credentials", "server", name, "error", err)
		}
	}
}

func (registry *Registry) setAuthURL(name string, generation uint64, url string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	entry, ok := registry.servers[name]
	if !ok || entry.generation != generation || registry.closing {
		return
	}
	entry.authURL = url
	entry.status = domainmcp.StatusAuthenticating
	entry.lastChange = time.Now()
	registry.emitLocked(domainmcp.Event{Type: "auth_url", Name: name, Status: entry.status, AuthURL: url})
}

func (registry *Registry) persistOAuthToken(name string, handler auth.OAuthHandler) error {
	if handler == nil {
		return nil
	}
	tokenSource, err := handler.TokenSource(registry.ctx)
	if err != nil {
		return fmt.Errorf("get token source: %w", err)
	}
	if tokenSource == nil {
		return nil
	}
	token, err := tokenSource.Token()
	if err != nil {
		return fmt.Errorf("read access token: %w", err)
	}
	if token == nil || token.AccessToken == "" {
		return nil
	}
	data, err := json.MarshalIndent(tokenFile{Token: *token}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode token: %w", err)
	}
	path := registry.tokenPath(name)
	tmp, err := os.CreateTemp(registry.authDir, ".mcp-auth-*.json")
	if err != nil {
		return fmt.Errorf("create token temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("set token permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write token: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync token: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close token: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace token: %w", err)
	}
	return nil
}

func (registry *Registry) loadOAuthToken(name string) (*oauth2.Token, error) {
	path := registry.tokenPath(name)
	if err := os.Chmod(path, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
		registry.logger.Warn("failed to secure MCP OAuth credentials", "server", name, "error", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file tokenFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	return &file.Token, nil
}

func (registry *Registry) tokenPath(name string) string {
	return filepath.Join(registry.authDir, name+".json")
}

func (registry *Registry) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	registry.closeOnce.Do(func() {
		registry.mu.Lock()
		registry.closing = true
		sessions := make([]struct {
			session *mcp.ClientSession
			cancel  context.CancelFunc
		}, 0, len(registry.servers))
		for _, entry := range registry.servers {
			session, sessionCancel := registry.invalidateLocked(entry)
			if session != nil || sessionCancel != nil {
				sessions = append(sessions, struct {
					session *mcp.ClientSession
					cancel  context.CancelFunc
				}{session: session, cancel: sessionCancel})
			}
			entry.status = domainmcp.StatusClosing
			entry.lastChange = time.Now()
		}
		registry.mu.Unlock()
		var sessionWait sync.WaitGroup
		for _, handle := range sessions {
			sessionWait.Add(1)
			registry.startTaskForShutdown()
			go func(session *mcp.ClientSession, sessionCancel context.CancelFunc) {
				defer sessionWait.Done()
				defer registry.wait.Done()
				closeMCPConnection(session, sessionCancel, registry.logger)
			}(handle.session, handle.cancel)
		}
		// Cancel pending connection/login work only after active sessions have
		// been given a chance to perform their graceful protocol close.
		go func() {
			sessionWait.Wait()
			registry.sessionBaseCancel()
			registry.cancel()
		}()
	})
	done := make(chan struct{})
	go func() {
		registry.wait.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("mcp: shutdown: %w", ctx.Err())
	}
}

// Close provides an io.Closer-style shutdown entry point for callers that do
// not need to choose their own deadline.
func (registry *Registry) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return registry.Shutdown(ctx)
}

func (registry *Registry) startTaskForShutdown() {
	registry.wait.Add(1)
}

func requiresAuth(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unauthorized") || strings.Contains(message, "forbidden") || strings.Contains(message, "authentication") || strings.Contains(message, "401") || strings.Contains(message, "403")
}

func expandMap(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = os.ExpandEnv(value)
	}
	return result
}

func envList(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	sort.Strings(result)
	return result
}

func bearerValue(token *oauth2.Token) string {
	if token.TokenType == "" {
		return "Bearer " + token.AccessToken
	}
	return token.TokenType + " " + token.AccessToken
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (transport *headerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	base := transport.base
	if base == nil {
		base = http.DefaultTransport
	}
	clone := request.Clone(request.Context())
	for key, value := range transport.headers {
		if strings.EqualFold(key, "Authorization") && clone.Header.Get("Authorization") != "" {
			continue
		}
		clone.Header.Set(key, value)
	}
	return base.RoundTrip(clone)
}

var _ agentloop.Tool = (*remoteTool)(nil)
