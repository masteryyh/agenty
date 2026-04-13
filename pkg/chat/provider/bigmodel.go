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

package provider

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"github.com/samber/lo"
)

const (
	bigModelDefaultBaseURL = "https://open.bigmodel.cn/api/paas/v4"
	bigModelToolTypeFunc   = "function"
)

type BigModelProvider struct {
	httpClient *http.Client
}

func NewBigModelProvider() *BigModelProvider {
	return &BigModelProvider{
		httpClient: &http.Client{},
	}
}

func (p *BigModelProvider) Name() string {
	return "bigmodel"
}

type bigModelRequest struct {
	Model    string            `json:"model"`
	Messages []bigModelMessage `json:"messages"`
	Tools    []bigModelTool    `json:"tools,omitempty"`
	Thinking *bigModelThinking `json:"thinking,omitempty"`
	Stream   bool              `json:"stream,omitempty"`
}

type bigModelMessage struct {
	Role             models.MessageRole `json:"role"`
	Content          string             `json:"content"`
	ToolCalls        []bigModelToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string             `json:"tool_call_id,omitempty"`
	ReasoningContent string             `json:"reasoning_content,omitempty"`
}

type bigModelToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function bigModelToolCallFunc `json:"function"`
}

type bigModelToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type bigModelTool struct {
	Type     string              `json:"type"`
	Function bigModelToolFuncDef `json:"function"`
}

type bigModelToolFuncDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type bigModelThinking struct {
	Type          string `json:"type"`
	ClearThinking bool   `json:"clear_thinking"`
}

type bigModelResponse struct {
	Choices []bigModelChoice `json:"choices"`
	Usage   bigModelUsage    `json:"usage"`
	Error   *bigModelError   `json:"error,omitempty"`
}

type bigModelChoice struct {
	Message bigModelMessage `json:"message"`
}

type bigModelUsage struct {
	TotalTokens int64 `json:"total_tokens"`
}

type bigModelError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

type bigModelStreamChunk struct {
	Choices []bigModelStreamChoice `json:"choices"`
	Usage   *bigModelUsage         `json:"usage,omitempty"`
}

type bigModelStreamChoice struct {
	Delta bigModelStreamDelta `json:"delta"`
}

type bigModelStreamDelta struct {
	Content          string             `json:"content,omitempty"`
	ReasoningContent string             `json:"reasoning_content,omitempty"`
	ToolCalls        []bigModelToolCall `json:"tool_calls,omitempty"`
	Role             string             `json:"role,omitempty"`
}

func buildBigModelMessages(messages []Message) []bigModelMessage {
	return lo.FilterMap(messages, func(msg Message, _ int) (bigModelMessage, bool) {
		switch msg.Role {
		case models.RoleUser:
			return bigModelMessage{
				Role:    models.RoleUser,
				Content: msg.Content,
			}, true
		case models.RoleAssistant:
			bm := bigModelMessage{
				Role:             models.RoleAssistant,
				Content:          msg.Content,
				ReasoningContent: msg.ReasoningContent,
			}
			if len(msg.ToolCalls) > 0 {
				bm.ToolCalls = lo.Map(msg.ToolCalls, func(tc models.ToolCall, _ int) bigModelToolCall {
					return bigModelToolCall{
						ID:   tc.ID,
						Type: bigModelToolTypeFunc,
						Function: bigModelToolCallFunc{
							Name:      tc.Name,
							Arguments: string(tc.Arguments),
						},
					}
				})
			}
			return bm, true
		case models.RoleTool:
			if msg.ToolResult != nil {
				return bigModelMessage{
					Role:       models.RoleTool,
					Content:    msg.ToolResult.Content,
					ToolCallID: msg.ToolResult.CallID,
				}, true
			}
			return bigModelMessage{}, false
		case models.RoleSystem:
			return bigModelMessage{
				Role:    models.RoleSystem,
				Content: msg.Content,
			}, true
		default:
			return bigModelMessage{}, false
		}
	})
}

func buildBigModelTools(defs []tools.ToolDefinition) []bigModelTool {
	return lo.Map(defs, func(def tools.ToolDefinition, _ int) bigModelTool {
		properties := make(map[string]any)
		for name, prop := range def.Parameters.Properties {
			properties[name] = prop.ToMap()
		}
		return bigModelTool{
			Type: bigModelToolTypeFunc,
			Function: bigModelToolFuncDef{
				Name:        def.Name,
				Description: def.Description,
				Parameters: map[string]any{
					"type":       def.Parameters.Type,
					"properties": properties,
					"required":   def.Parameters.Required,
				},
			},
		}
	})
}

func (p *BigModelProvider) buildRequest(req *ChatRequest, stream bool) bigModelRequest {
	messages := buildBigModelMessages(req.Messages)
	apiReq := bigModelRequest{
		Model:    req.Model,
		Messages: messages,
		Stream:   stream,
	}

	thinkingType := "enabled"
	if !req.Thinking {
		thinkingType = "disabled"
	}
	apiReq.Thinking = &bigModelThinking{
		Type:          thinkingType,
		ClearThinking: req.BigModelClearThinking,
	}

	if len(req.Tools) > 0 {
		apiReq.Tools = buildBigModelTools(req.Tools)
	}

	return apiReq
}

func (p *BigModelProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = bigModelDefaultBaseURL
	}

	apiReq := p.buildRequest(req, false)

	body, err := json.Marshal(apiReq)
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

	var apiResp bigModelResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
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
		result.ReasoningContent = choice.Message.ReasoningContent

		if len(choice.Message.ToolCalls) > 0 {
			result.ToolCalls = lo.Map(choice.Message.ToolCalls, func(tc bigModelToolCall, _ int) models.ToolCall {
				return models.ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				}
			})
		}
	}

	return result, nil
}

func (p *BigModelProvider) StreamChat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = bigModelDefaultBaseURL
	}

	apiReq := p.buildRequest(req, true)

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		defer httpResp.Body.Close()
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", httpResp.StatusCode, string(respBody))
	}

	ch := make(chan StreamEvent, 64)

	safe.GoOnce("bigmodel-stream", func() {
		defer close(ch)
		defer httpResp.Body.Close()

		type toolCallAccum struct {
			id          string
			name        string
			argsBuilder strings.Builder
		}

		var contentBuilder strings.Builder
		var reasoningBuilder strings.Builder
		tcMap := make(map[int]*toolCallAccum)
		tcKeys := make([]int, 0)
		var totalTokens int64

		scanner := bufio.NewScanner(httpResp.Body)
		scanner.Buffer(make([]byte, 512*1024), 512*1024)
		for scanner.Scan() {
			line := scanner.Text()

			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var chunk bigModelStreamChunk
			if err := json.UnmarshalString(data, &chunk); err != nil {
				continue
			}

			if chunk.Usage != nil && chunk.Usage.TotalTokens > 0 {
				totalTokens = chunk.Usage.TotalTokens
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta

			if delta.Content != "" {
				ch <- StreamEvent{
					Type:    EventContentDelta,
					Content: delta.Content,
				}
				contentBuilder.WriteString(delta.Content)
			}

			if delta.ReasoningContent != "" {
				ch <- StreamEvent{
					Type:      EventReasoningDelta,
					Reasoning: delta.ReasoningContent,
				}
				reasoningBuilder.WriteString(delta.ReasoningContent)
			}

			for i, tc := range delta.ToolCalls {
				acc, ok := tcMap[i]
				if !ok {
					acc = &toolCallAccum{
						id:   tc.ID,
						name: tc.Function.Name,
					}
					tcMap[i] = acc
					tcKeys = append(tcKeys, i)
					if tc.ID != "" {
						ch <- StreamEvent{
							Type:     EventToolCallStart,
							ToolCall: &models.ToolCall{ID: tc.ID, Name: tc.Function.Name},
						}
					}
				}
				if tc.ID != "" && acc.id == "" {
					acc.id = tc.ID
				}
				if tc.Function.Name != "" && acc.name == "" {
					acc.name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					acc.argsBuilder.WriteString(tc.Function.Arguments)
					ch <- StreamEvent{
						Type:     EventToolCallDelta,
						ToolCall: &models.ToolCall{ID: acc.id, Name: acc.name, Arguments: tc.Function.Arguments},
					}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- StreamEvent{
				Type:  EventError,
				Error: fmt.Sprintf("stream read error: %v", err),
			}
			return
		}

		if totalTokens > 0 {
			ch <- StreamEvent{
				Type:  EventUsage,
				Usage: &StreamUsage{TotalTokens: totalTokens},
			}
		}

		var toolCalls []models.ToolCall
		for _, key := range tcKeys {
			acc := tcMap[key]
			tc := models.ToolCall{
				ID:        acc.id,
				Name:      acc.name,
				Arguments: acc.argsBuilder.String(),
			}
			toolCalls = append(toolCalls, tc)
			ch <- StreamEvent{
				Type:     EventToolCallDone,
				ToolCall: &tc,
			}
		}

		msg := &Message{
			Role:             models.RoleAssistant,
			Content:          contentBuilder.String(),
			ToolCalls:        toolCalls,
			ReasoningContent: reasoningBuilder.String(),
		}
		ch <- StreamEvent{
			Type:    EventMessageDone,
			Message: msg,
		}
	})

	return ch, nil
}
