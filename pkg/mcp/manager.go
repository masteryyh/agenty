/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	retry "github.com/avast/retry-go/v5"
	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/masteryyh/agenty/pkg/config"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/tools"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"github.com/masteryyh/agenty/pkg/utils/signal"
)

type ServerStatus string

const (
	StatusConnected    ServerStatus = "connected"
	StatusDisconnected ServerStatus = "disconnected"
	StatusConnecting   ServerStatus = "connecting"
	StatusError        ServerStatus = "error"
)

type mcpConnection struct {
	server    *models.MCPServer
	client    client.MCPClient
	status    ServerStatus
	lastPing  time.Time
	tools     []mcp.Tool
	resources []mcp.Resource
	prompts   []mcp.Prompt
	errMsg    string
}

type MCPManager struct {
	mu          sync.RWMutex
	connections map[uuid.UUID]*mcpConnection
	registry    *tools.Registry
	service     *services.MCPServerService
	cfg         *config.MCPConfig
}

var (
	globalManager *MCPManager
	managerOnce   sync.Once
)

func GetManager() *MCPManager {
	return globalManager
}

func InitManager(_ context.Context, registry *tools.Registry) *MCPManager {
	managerOnce.Do(func() {
		cfg := config.GetConfigManager().GetConfig().MCP
		if cfg == nil {
			cfg = &config.MCPConfig{HealthCheckInterval: 30, ConnectTimeout: 15}
		}

		globalManager = &MCPManager{
			connections: make(map[uuid.UUID]*mcpConnection),
			registry:    registry,
			service:     services.GetMCPServerService(),
			cfg:         cfg,
		}
	})
	return globalManager
}

func (m *MCPManager) Start() {
	ctx := signal.GetBaseContext()
	servers, err := m.service.ListAllEnabled(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to load mcp servers", "error", err)
		return
	}

	for i := range servers {
		s := servers[i]
		if err := m.connect(ctx, &s); err != nil {
			slog.ErrorContext(ctx, "failed to connect mcp server", "name", s.Name, "error", err)
		}
	}

	safe.GoSafeWithCtx("mcp-health-check", ctx, func(ctx context.Context) {
		ticker := time.NewTicker(time.Duration(m.cfg.HealthCheckInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.healthCheck(ctx)
			}
		}
	})
}

func (m *MCPManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, conn := range m.connections {
		if conn.client != nil {
			conn.client.Close()
		}
		m.registry.UnregisterByPrefix(fmt.Sprintf("mcp_%s_", conn.server.Name))
		delete(m.connections, id)
	}
}

func (m *MCPManager) Connect(serverID uuid.UUID) error {
	ctx := signal.GetBaseContext()

	m.mu.Lock()
	if existing, ok := m.connections[serverID]; ok {
		if existing.client != nil {
			existing.client.Close()
		}
		m.registry.UnregisterByPrefix(fmt.Sprintf("mcp_%s_", existing.server.Name))
		delete(m.connections, serverID)
	}
	m.mu.Unlock()

	server, err := m.service.GetMCPServerModel(ctx, serverID)
	if err != nil {
		return err
	}

	return m.connect(ctx, server)
}

func (m *MCPManager) Disconnect(serverID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, ok := m.connections[serverID]
	if !ok {
		return nil
	}

	if conn.client != nil {
		conn.client.Close()
	}
	m.registry.UnregisterByPrefix(fmt.Sprintf("mcp_%s_", conn.server.Name))
	conn.client = nil
	conn.status = StatusDisconnected
	conn.errMsg = ""

	return nil
}

func (m *MCPManager) GetStatus(serverID uuid.UUID) (ServerStatus, string, []string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, ok := m.connections[serverID]
	if !ok {
		return StatusDisconnected, "", nil
	}

	toolNames := make([]string, len(conn.tools))
	for i, t := range conn.tools {
		toolNames[i] = t.Name
	}

	return conn.status, conn.errMsg, toolNames
}

func (m *MCPManager) GetAllStatuses() map[uuid.UUID]*models.MCPServerDto {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[uuid.UUID]*models.MCPServerDto, len(m.connections))
	for id, conn := range m.connections {
		dto := conn.server.ToDto()
		dto.Status = string(conn.status)
		dto.Error = conn.errMsg
		toolNames := make([]string, len(conn.tools))
		for i, t := range conn.tools {
			toolNames[i] = t.Name
		}
		dto.Tools = toolNames
		result[id] = dto
	}

	return result
}

func (m *MCPManager) connect(ctx context.Context, server *models.MCPServer) error {
	m.mu.Lock()
	m.connections[server.ID] = &mcpConnection{
		server: server,
		status: StatusConnecting,
	}
	m.mu.Unlock()

	connectCtx, cancel := context.WithTimeout(ctx, time.Duration(m.cfg.ConnectTimeout)*time.Second)
	defer cancel()

	t, err := m.createTransport(server)
	if err != nil {
		m.setError(server.ID, err)
		return err
	}

	c := client.NewClient(t)
	if err := c.Start(connectCtx); err != nil {
		m.setError(server.ID, err)
		return fmt.Errorf("failed to start mcp client for %s: %w", server.Name, err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "agenty",
		Version: "0.0.1",
	}

	_, err = c.Initialize(connectCtx, initReq)
	if err != nil {
		c.Close()
		m.setError(server.ID, err)
		return fmt.Errorf("failed to initialize mcp connection for %s: %w", server.Name, err)
	}

	if err := m.discoverAndRegister(connectCtx, server, c); err != nil {
		c.Close()
		m.setError(server.ID, err)
		return err
	}

	m.mu.Lock()
	if conn, ok := m.connections[server.ID]; ok {
		conn.client = c
		conn.status = StatusConnected
		conn.lastPing = time.Now()
		conn.errMsg = ""

		slog.InfoContext(ctx, "mcp server connected", "name", server.Name, "tools", len(m.connections[server.ID].tools))
	}
	m.mu.Unlock()
	return nil
}

func (m *MCPManager) discoverAndRegister(ctx context.Context, server *models.MCPServer, c client.MCPClient) error {
	toolsResult, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return fmt.Errorf("failed to list tools from %s: %w", server.Name, err)
	}

	var mcpTools []mcp.Tool
	if toolsResult != nil {
		mcpTools = toolsResult.Tools
	}

	var mcpResources []mcp.Resource
	resourcesResult, err := c.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		slog.WarnContext(ctx, "failed to list resources from mcp server", "name", server.Name, "error", err)
	} else if resourcesResult != nil {
		mcpResources = resourcesResult.Resources
	}

	var mcpPrompts []mcp.Prompt
	promptsResult, err := c.ListPrompts(ctx, mcp.ListPromptsRequest{})
	if err != nil {
		slog.WarnContext(ctx, "failed to list prompts from mcp server", "name", server.Name, "error", err)
	} else if promptsResult != nil {
		mcpPrompts = promptsResult.Prompts
	}

	m.mu.Lock()
	if conn, ok := m.connections[server.ID]; ok {
		conn.tools = mcpTools
		conn.resources = mcpResources
		conn.prompts = mcpPrompts
	}
	m.mu.Unlock()

	prefix := fmt.Sprintf("mcp_%s_", server.Name)
	m.registry.UnregisterByPrefix(prefix)

	for _, t := range mcpTools {
		m.registry.Register(NewMCPTool(server.Name, t, c))
	}

	if len(mcpResources) > 0 {
		m.registry.Register(NewMCPResourceTool(server.Name, c, mcpResources))
	}

	return nil
}

func (m *MCPManager) createTransport(server *models.MCPServer) (transport.Interface, error) {
	switch server.Transport {
	case models.MCPTransportStdio:
		var args []string
		if len(server.Args) > 0 {
			if err := json.Unmarshal(server.Args, &args); err != nil {
				return nil, fmt.Errorf("failed to parse mcp server args: %w", err)
			}
		}
		var envMap map[string]string
		if len(server.Env) > 0 {
			if err := json.Unmarshal(server.Env, &envMap); err != nil {
				return nil, fmt.Errorf("failed to parse mcp server env: %w", err)
			}
		}
		var envSlice []string
		for k, v := range envMap {
			envSlice = append(envSlice, fmt.Sprintf("%s=%s", k, v))
		}
		return transport.NewStdio(server.Command, envSlice, args...), nil

	case models.MCPTransportSSE:
		var headers map[string]string
		if len(server.Headers) > 0 {
			if err := json.Unmarshal(server.Headers, &headers); err != nil {
				return nil, fmt.Errorf("failed to parse mcp server headers: %w", err)
			}
		}
		var opts []transport.ClientOption
		if len(headers) > 0 {
			opts = append(opts, transport.WithHeaders(headers))
		}
		t, err := transport.NewSSE(server.URL, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create sse transport: %w", err)
		}
		return t, nil

	case models.MCPTransportStreamableHTTP:
		var headers map[string]string
		if len(server.Headers) > 0 {
			if err := json.Unmarshal(server.Headers, &headers); err != nil {
				return nil, fmt.Errorf("failed to parse mcp server headers: %w", err)
			}
		}
		var opts []transport.StreamableHTTPCOption
		if len(headers) > 0 {
			opts = append(opts, transport.WithHTTPHeaders(headers))
		}
		t, err := transport.NewStreamableHTTP(server.URL, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create streamable http transport: %w", err)
		}
		return t, nil

	default:
		return nil, fmt.Errorf("unsupported transport type: %s", server.Transport)
	}
}

func (m *MCPManager) healthCheck(ctx context.Context) {
	m.mu.RLock()
	ids := make([]uuid.UUID, 0, len(m.connections))
	for id, conn := range m.connections {
		if conn.status == StatusConnected && conn.client != nil {
			ids = append(ids, id)
		}
	}
	m.mu.RUnlock()

	for _, id := range ids {
		m.mu.RLock()
		conn, ok := m.connections[id]
		if !ok {
			m.mu.RUnlock()
			continue
		}
		c := conn.client
		name := conn.server.Name
		m.mu.RUnlock()

		pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := c.Ping(pingCtx)
		cancel()

		if err != nil {
			slog.WarnContext(ctx, "mcp server health check failed", "name", name, "error", err)
			m.mu.Lock()
			if conn, ok := m.connections[id]; ok {
				conn.status = StatusError
				conn.errMsg = err.Error()
			}
			m.mu.Unlock()

			m.tryReconnect(ctx, id)
		} else {
			m.mu.Lock()
			if conn, ok := m.connections[id]; ok {
				conn.lastPing = time.Now()
			}
			m.mu.Unlock()
		}
	}
}

func (m *MCPManager) tryReconnect(ctx context.Context, serverID uuid.UUID) {
	m.mu.RLock()
	conn, ok := m.connections[serverID]
	if !ok {
		m.mu.RUnlock()
		return
	}
	server := conn.server
	name := server.Name
	if conn.client != nil {
		conn.client.Close()
	}
	m.mu.RUnlock()

	m.registry.UnregisterByPrefix(fmt.Sprintf("mcp_%s_", name))

	retrier := retry.New(
		retry.Attempts(5),
		retry.Delay(time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.MaxDelay(30*time.Second),
		retry.Context(ctx),
		retry.OnRetry(func(attempt uint, err error) {
			slog.WarnContext(ctx, "mcp server reconnection failed", "name", name, "attempt", attempt+1, "error", err)
		}),
	)

	if err := retrier.Do(func() error {
		return m.connect(ctx, server)
	}); err != nil {
		slog.ErrorContext(ctx, "mcp server reconnection failed after max retries", "name", name, "error", err)
		m.mu.Lock()
		if conn, ok := m.connections[serverID]; ok {
			conn.status = StatusDisconnected
			conn.client = nil
		}
		m.mu.Unlock()
		return
	}

	slog.InfoContext(ctx, "mcp server reconnected", "name", name)
}

func (m *MCPManager) setError(serverID uuid.UUID, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if conn, ok := m.connections[serverID]; ok {
		conn.status = StatusError
		conn.errMsg = err.Error()
	}
}
