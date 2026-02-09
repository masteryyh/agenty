package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/masteryyh/agenty/pkg/chat/tools"
)

type AnthropicProvider struct {
	httpClient *http.Client
}

func NewAnthropicProvider() *AnthropicProvider {
	return &AnthropicProvider{
		httpClient: &http.Client{},
	}
}

func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

type anthropicRequest struct {
	Model     string                    `json:"model"`
	MaxTokens int                       `json:"max_tokens"`
	Messages  []anthropicMessage        `json:"messages"`
	Tools     []anthropicToolDefinition `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string        `json:"role"`
	Content any           `json:"content"`
}

type anthropicTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicToolUseContent struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type anthropicToolResultContent struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

type anthropicToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]any         `json:"input_schema"`
}

type anthropicResponse struct {
	ID      string                   `json:"id"`
	Content []anthropicContentBlock  `json:"content"`
	Usage   anthropicUsage           `json:"usage"`
	Error   *anthropicError          `json:"error,omitempty"`
}

type anthropicContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

type anthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (p *AnthropicProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}

	messages := buildAnthropicMessages(req.Messages)
	apiReq := anthropicRequest{
		Model:     req.Model,
		MaxTokens: 4096,
		Messages:  messages,
	}

	if len(req.Tools) > 0 {
		apiReq.Tools = buildAnthropicTools(req.Tools)
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", req.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", httpResp.StatusCode, string(respBody))
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("API error: %s", apiResp.Error.Message)
	}

	result := &ChatResponse{
		TotalToken: apiResp.Usage.InputTokens + apiResp.Usage.OutputTokens,
	}

	var textParts []string
	for _, block := range apiResp.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, tools.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: block.Input,
			})
		}
	}
	if len(textParts) > 0 {
		result.Content = textParts[0]
		for _, part := range textParts[1:] {
			result.Content += "\n" + part
		}
	}

	return result, nil
}

func buildAnthropicMessages(messages []Message) []anthropicMessage {
	var result []anthropicMessage
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			result = append(result, anthropicMessage{
				Role:    "user",
				Content: msg.Content,
			})
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				var content []any
				if msg.Content != "" {
					content = append(content, anthropicTextContent{
						Type: "text",
						Text: msg.Content,
					})
				}
				for _, tc := range msg.ToolCalls {
					content = append(content, anthropicToolUseContent{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Name,
						Input: tc.Arguments,
					})
				}
				result = append(result, anthropicMessage{
					Role:    "assistant",
					Content: content,
				})
			} else {
				result = append(result, anthropicMessage{
					Role:    "assistant",
					Content: msg.Content,
				})
			}
		case "tool":
			if msg.ToolResult != nil {
				result = append(result, anthropicMessage{
					Role: "user",
					Content: []anthropicToolResultContent{
						{
							Type:      "tool_result",
							ToolUseID: msg.ToolResult.CallID,
							Content:   msg.ToolResult.Content,
							IsError:   msg.ToolResult.IsError,
						},
					},
				})
			}
		}
	}
	return result
}

func buildAnthropicTools(defs []tools.ToolDefinition) []anthropicToolDefinition {
	result := make([]anthropicToolDefinition, len(defs))
	for i, def := range defs {
		properties := make(map[string]any)
		for name, prop := range def.Parameters.Properties {
			properties[name] = map[string]any{
				"type":        prop.Type,
				"description": prop.Description,
			}
		}
		result[i] = anthropicToolDefinition{
			Name:        def.Name,
			Description: def.Description,
			InputSchema: map[string]any{
				"type":       def.Parameters.Type,
				"properties": properties,
				"required":   def.Parameters.Required,
			},
		}
	}
	return result
}
