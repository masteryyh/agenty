package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/chat/tools"
)

type KimiProvider struct {
	httpClient *http.Client
}

func NewKimiProvider() *KimiProvider {
	return &KimiProvider{
		httpClient: &http.Client{},
	}
}

func (p *KimiProvider) Name() string {
	return "kimi"
}

type kimiRequest struct {
	Model    string             `json:"model"`
	Messages []kimiMessage      `json:"messages"`
	Tools    []kimiTool         `json:"tools,omitempty"`
	Thinking *kimiThinking      `json:"thinking,omitempty"`
}

type kimiMessage struct {
	Role             string         `json:"role"`
	Content          string         `json:"content"`
	ToolCalls        []kimiToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
}

type kimiToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function kimiToolFunction `json:"function"`
}

type kimiToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type kimiTool struct {
	Type     string               `json:"type"`
	Function kimiToolFunctionDef  `json:"function"`
}

type kimiToolFunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type kimiThinking struct {
	Type string `json:"type"`
}

type kimiResponse struct {
	Choices []kimiChoice `json:"choices"`
	Usage   kimiUsage    `json:"usage"`
	Error   *kimiError   `json:"error,omitempty"`
}

type kimiChoice struct {
	Message kimiMessage `json:"message"`
}

type kimiUsage struct {
	TotalTokens int64 `json:"total_tokens"`
}

type kimiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (p *KimiProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = "https://api.moonshot.ai/v1"
	}

	messages := buildKimiMessages(req.Messages)
	apiReq := kimiRequest{
		Model:    req.Model,
		Messages: messages,
	}

	isThinkingModel := req.Model == "kimi-k2-thinking" || req.Model == "kimi-k2.5" ||
		strings.HasPrefix(req.Model, "kimi-k2")
	if isThinkingModel {
		apiReq.Thinking = &kimiThinking{Type: "enabled"}
	}

	if len(req.Tools) > 0 {
		apiReq.Tools = buildKimiTools(req.Tools)
	}

	body, err := sonic.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)

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

	var apiResp kimiResponse
	if err := sonic.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("API error: %s", apiResp.Error.Message)
	}

	result := &ChatResponse{
		TotalToken: apiResp.Usage.TotalTokens,
	}

	if len(apiResp.Choices) > 0 {
		choice := apiResp.Choices[0]
		result.Content = choice.Message.Content

		for _, tc := range choice.Message.ToolCalls {
			result.ToolCalls = append(result.ToolCalls, tools.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: json.RawMessage(tc.Function.Arguments),
			})
		}
	}

	return result, nil
}

func buildKimiMessages(messages []Message) []kimiMessage {
	result := make([]kimiMessage, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			result = append(result, kimiMessage{
				Role:    "user",
				Content: msg.Content,
			})
		case "assistant":
			km := kimiMessage{
				Role:    "assistant",
				Content: msg.Content,
			}
			for _, tc := range msg.ToolCalls {
				km.ToolCalls = append(km.ToolCalls, kimiToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: kimiToolFunction{
						Name:      tc.Name,
						Arguments: string(tc.Arguments),
					},
				})
			}
			result = append(result, km)
		case "tool":
			if msg.ToolResult != nil {
				result = append(result, kimiMessage{
					Role:       "tool",
					Content:    msg.ToolResult.Content,
					ToolCallID: msg.ToolResult.CallID,
				})
			}
		case "system":
			result = append(result, kimiMessage{
				Role:    "system",
				Content: msg.Content,
			})
		}
	}
	return result
}

func buildKimiTools(defs []tools.ToolDefinition) []kimiTool {
	result := make([]kimiTool, len(defs))
	for i, def := range defs {
		properties := make(map[string]any)
		for name, prop := range def.Parameters.Properties {
			properties[name] = map[string]any{
				"type":        prop.Type,
				"description": prop.Description,
			}
		}
		result[i] = kimiTool{
			Type: "function",
			Function: kimiToolFunctionDef{
				Name:        def.Name,
				Description: def.Description,
				Parameters: map[string]any{
					"type":       def.Parameters.Type,
					"properties": properties,
					"required":   def.Parameters.Required,
				},
			},
		}
	}
	return result
}
