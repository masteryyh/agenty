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

package api

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/chat/provider"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/pagination"
	"github.com/masteryyh/agenty/pkg/utils/response"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	username   string
	password   string
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func NewClientWithAuth(baseURL, username, password string) *Client {
	return &Client{
		baseURL:  baseURL,
		username: username,
		password: password,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func doRequest[T any](c *Client, method, path string, body any) (*T, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var apiResp response.GenericResponse[T]
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if apiResp.Code != 200 {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}
	return apiResp.Data, nil
}

func (c *Client) ListProviders(page, pageSize int) (*pagination.PagedResponse[models.ModelProviderDto], error) {
	return doRequest[pagination.PagedResponse[models.ModelProviderDto]](c, "GET", fmt.Sprintf("/api/v1/providers?page=%d&pageSize=%d", page, pageSize), nil)
}

func (c *Client) CreateProvider(dto *models.CreateModelProviderDto) (*models.ModelProviderDto, error) {
	return doRequest[models.ModelProviderDto](c, "POST", "/api/v1/providers", dto)
}

func (c *Client) UpdateProvider(providerID uuid.UUID, dto *models.UpdateModelProviderDto) (*models.ModelProviderDto, error) {
	return doRequest[models.ModelProviderDto](c, "PUT", fmt.Sprintf("/api/v1/providers/%s", providerID), dto)
}

func (c *Client) DeleteProvider(providerID uuid.UUID, force bool) error {
	path := fmt.Sprintf("/api/v1/providers/%s", providerID)
	if force {
		path += "?force=true"
	}
	_, err := doRequest[any](c, "DELETE", path, nil)
	return err
}

func (c *Client) ListModels(page, pageSize int) (*pagination.PagedResponse[models.ModelDto], error) {
	return doRequest[pagination.PagedResponse[models.ModelDto]](c, "GET", fmt.Sprintf("/api/v1/models?page=%d&pageSize=%d", page, pageSize), nil)
}

func (c *Client) CreateModel(dto *models.CreateModelDto) (*models.ModelDto, error) {
	return doRequest[models.ModelDto](c, "POST", "/api/v1/models", dto)
}

func (c *Client) GetDefaultModel() (*models.ModelDto, error) {
	return doRequest[models.ModelDto](c, "GET", "/api/v1/models/default", nil)
}

func (c *Client) UpdateModel(modelID uuid.UUID, dto *models.UpdateModelDto) error {
	_, err := doRequest[any](c, "PUT", fmt.Sprintf("/api/v1/models/%s", modelID), dto)
	return err
}

func (c *Client) DeleteModel(modelID uuid.UUID) error {
	_, err := doRequest[any](c, "DELETE", fmt.Sprintf("/api/v1/models/%s", modelID), nil)
	return err
}

func (c *Client) ListSessions(page, pageSize int) (*pagination.PagedResponse[models.ChatSessionDto], error) {
	return doRequest[pagination.PagedResponse[models.ChatSessionDto]](c, "GET", fmt.Sprintf("/api/v1/chats/sessions?page=%d&pageSize=%d", page, pageSize), nil)
}

func (c *Client) CreateSession(agentID uuid.UUID) (*models.ChatSessionDto, error) {
	return doRequest[models.ChatSessionDto](c, "POST", "/api/v1/chats/session", &models.CreateSessionDto{AgentID: agentID})
}

func (c *Client) GetSession(sessionID uuid.UUID) (*models.ChatSessionDto, error) {
	return doRequest[models.ChatSessionDto](c, "GET", fmt.Sprintf("/api/v1/chats/session/%s", sessionID), nil)
}

func (c *Client) GetLastSession() (*models.ChatSessionDto, error) {
	return doRequest[models.ChatSessionDto](c, "GET", "/api/v1/chats/session/last", nil)
}

func (c *Client) GetLastSessionByAgent(agentID uuid.UUID) (*models.ChatSessionDto, error) {
	return doRequest[models.ChatSessionDto](c, "GET", fmt.Sprintf("/api/v1/chats/session/last/%s", agentID), nil)
}

func (c *Client) Chat(sessionID uuid.UUID, dto *models.ChatDto) (*[]*models.ChatMessageDto, error) {
	return doRequest[[]*models.ChatMessageDto](c, "POST", fmt.Sprintf("/api/v1/chats/chat?sessionId=%s", sessionID), dto)
}

func (c *Client) StreamChat(ctx context.Context, sessionID uuid.UUID, dto *models.ChatDto, handler func(event provider.StreamEvent) error) error {
	jsonData, err := json.Marshal(dto)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+fmt.Sprintf("/api/v1/chats/stream?sessionId=%s", sessionID), bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	sseClient := &http.Client{Timeout: 0}
	resp, err := sseClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var currentData string
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "data:") {
			currentData = strings.TrimPrefix(line, "data:")
			currentData = strings.TrimSpace(currentData)
			continue
		}

		if line == "" && currentData != "" {
			var evt provider.StreamEvent
			if err := json.UnmarshalString(currentData, &evt); err != nil {
				currentData = ""
				continue
			}
			currentData = ""

			if err := handler(evt); err != nil {
				return err
			}

			if evt.Type == provider.EventDone {
				return nil
			}
		}
	}

	return scanner.Err()
}

func (c *Client) GetModelThinkingLevels(modelID uuid.UUID) (*[]string, error) {
	return doRequest[[]string](c, "GET", fmt.Sprintf("/api/v1/models/%s/thinking-levels", modelID), nil)
}

func (c *Client) ListAgents(page, pageSize int) (*pagination.PagedResponse[models.AgentDto], error) {
	return doRequest[pagination.PagedResponse[models.AgentDto]](c, "GET", fmt.Sprintf("/api/v1/agents?page=%d&pageSize=%d", page, pageSize), nil)
}

func (c *Client) GetAgent(agentID uuid.UUID) (*models.AgentDto, error) {
	return doRequest[models.AgentDto](c, "GET", fmt.Sprintf("/api/v1/agents/%s", agentID), nil)
}

func (c *Client) CreateAgent(dto *models.CreateAgentDto) (*models.AgentDto, error) {
	return doRequest[models.AgentDto](c, "POST", "/api/v1/agents", dto)
}

func (c *Client) UpdateAgent(agentID uuid.UUID, dto *models.UpdateAgentDto) error {
	_, err := doRequest[any](c, "PUT", fmt.Sprintf("/api/v1/agents/%s", agentID), dto)
	return err
}

func (c *Client) DeleteAgent(agentID uuid.UUID) error {
	_, err := doRequest[any](c, "DELETE", fmt.Sprintf("/api/v1/agents/%s", agentID), nil)
	return err
}

func (c *Client) ListMCPServers(page, pageSize int) (*pagination.PagedResponse[models.MCPServerDto], error) {
	return doRequest[pagination.PagedResponse[models.MCPServerDto]](c, "GET", fmt.Sprintf("/api/v1/mcp/servers?page=%d&pageSize=%d", page, pageSize), nil)
}

func (c *Client) CreateMCPServer(dto *models.CreateMCPServerDto) (*models.MCPServerDto, error) {
	return doRequest[models.MCPServerDto](c, "POST", "/api/v1/mcp/servers", dto)
}

func (c *Client) UpdateMCPServer(serverID uuid.UUID, dto *models.UpdateMCPServerDto) (*models.MCPServerDto, error) {
	return doRequest[models.MCPServerDto](c, "PUT", fmt.Sprintf("/api/v1/mcp/servers/%s", serverID), dto)
}

func (c *Client) DeleteMCPServer(serverID uuid.UUID) error {
	_, err := doRequest[any](c, "DELETE", fmt.Sprintf("/api/v1/mcp/servers/%s", serverID), nil)
	return err
}

func (c *Client) ConnectMCPServer(serverID uuid.UUID) error {
	_, err := doRequest[any](c, "POST", fmt.Sprintf("/api/v1/mcp/servers/%s/connect", serverID), nil)
	return err
}

func (c *Client) DisconnectMCPServer(serverID uuid.UUID) error {
	_, err := doRequest[any](c, "POST", fmt.Sprintf("/api/v1/mcp/servers/%s/disconnect", serverID), nil)
	return err
}
