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

package providers

import (
	"context"
	"fmt"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/tools"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"github.com/samber/lo"
)

const (
	kimiDefaultBaseURL = "https://api.moonshot.ai/v1"
	kimiToolTypeFunc   = "function"
	kimiThinkingOff    = "disabled"
)

type KimiProvider struct{}

func NewKimiProvider() *KimiProvider {
	return &KimiProvider{}
}

func (p *KimiProvider) Name() string {
	return "kimi"
}

type kimiRequest struct {
	Model          string              `json:"model"`
	Messages       []kimiMessage       `json:"messages"`
	Tools          []kimiTool          `json:"tools,omitempty"`
	Thinking       *kimiThinking       `json:"thinking,omitempty"`
	ResponseFormat *kimiResponseFormat `json:"response_format,omitempty"`
	Stream         bool                `json:"stream,omitempty"`
}

type kimiResponseFormat struct {
	Type string `json:"type"`
}

type kimiMessage struct {
	Role             models.MessageRole `json:"role"`
	Content          string             `json:"content"`
	ToolCalls        []kimiToolCall     `json:"tool_calls,omitempty"`
	ToolCallID       string             `json:"tool_call_id,omitempty"`
	ReasoningContent string             `json:"reasoning_content,omitempty"`
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
	Type     string              `json:"type"`
	Function kimiToolFunctionDef `json:"function"`
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
		baseURL = kimiDefaultBaseURL
	}

	apiReq := p.buildKimiRequest(req, false)

	apiResp, err := conn.Post[kimiResponse](ctx, conn.HTTPRequest{
		URL:     baseURL + "/chat/completions",
		Headers: map[string]string{"Authorization": "Bearer " + req.APIKey},
		Body:    apiReq,
	})
	if err != nil {
		return nil, err
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
			result.ToolCalls = lo.Map(choice.Message.ToolCalls, func(tc kimiToolCall, _ int) models.ToolCall {
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

func buildKimiMessages(messages []Message) []kimiMessage {
	return lo.FilterMap(messages, func(msg Message, _ int) (kimiMessage, bool) {
		switch msg.Role {
		case models.RoleUser:
			return kimiMessage{
				Role:             models.RoleUser,
				Content:          msg.Content,
				ReasoningContent: msg.ReasoningContent,
			}, true
		case models.RoleAssistant:
			km := kimiMessage{
				Role:             models.RoleAssistant,
				Content:          msg.Content,
				ReasoningContent: msg.ReasoningContent,
			}
			if len(msg.ToolCalls) > 0 {
				km.ToolCalls = lo.Map(msg.ToolCalls, func(tc models.ToolCall, _ int) kimiToolCall {
					return kimiToolCall{
						ID:   tc.ID,
						Type: kimiToolTypeFunc,
						Function: kimiToolFunction{
							Name:      tc.Name,
							Arguments: string(tc.Arguments),
						},
					}
				})
			}
			return km, true
		case models.RoleTool:
			if msg.ToolResult != nil {
				return kimiMessage{
					Role:             models.RoleTool,
					Content:          msg.ToolResult.Content,
					ToolCallID:       msg.ToolResult.CallID,
					ReasoningContent: msg.ReasoningContent,
				}, true
			}
			return kimiMessage{}, false
		case models.RoleSystem:
			return kimiMessage{
				Role:    models.RoleSystem,
				Content: msg.Content,
			}, true
		default:
			return kimiMessage{}, false
		}
	})
}

func buildKimiTools(defs []tools.ToolDefinition) []kimiTool {
	return lo.Map(defs, func(def tools.ToolDefinition, _ int) kimiTool {
		properties := make(map[string]any)
		for name, prop := range def.Parameters.Properties {
			properties[name] = prop.ToMap()
		}
		return kimiTool{
			Type: kimiToolTypeFunc,
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
	})
}

type kimiStreamChunk struct {
	Choices []kimiStreamChoice `json:"choices"`
	Usage   *kimiUsage         `json:"usage,omitempty"`
}

type kimiStreamChoice struct {
	Delta kimiStreamDelta `json:"delta"`
}

type kimiStreamDelta struct {
	Content          string         `json:"content,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        []kimiToolCall `json:"tool_calls,omitempty"`
	Role             string         `json:"role,omitempty"`
}

func (p *KimiProvider) buildKimiRequest(req *ChatRequest, stream bool) kimiRequest {
	messages := buildKimiMessages(req.Messages)
	apiReq := kimiRequest{
		Model:    req.Model,
		Messages: messages,
		Stream:   stream,
	}

	isThinkingModel := req.Model == "kimi-k2-thinking" || req.Model == "kimi-k2.5" ||
		strings.HasPrefix(req.Model, "kimi-k2")
	if !isThinkingModel {
		apiReq.Thinking = &kimiThinking{Type: kimiThinkingOff}
	}

	if len(req.Tools) > 0 {
		apiReq.Tools = buildKimiTools(req.Tools)
	}

	if req.ResponseFormat != nil && req.ResponseFormat.Type == "json_object" {
		apiReq.ResponseFormat = &kimiResponseFormat{Type: "json_object"}
	}

	return apiReq
}

func (p *KimiProvider) StreamChat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = kimiDefaultBaseURL
	}

	apiReq := p.buildKimiRequest(req, true)

	lines, err := conn.PostSSE(ctx, conn.HTTPRequest{
		URL:     baseURL + "/chat/completions",
		Headers: map[string]string{"Authorization": "Bearer " + req.APIKey},
		Body:    apiReq,
	})
	if err != nil {
		return nil, err
	}

	ch := make(chan StreamEvent, 64)

	safe.GoOnce("kimi-stream", func() {
		defer close(ch)

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

		for evt := range lines {
			if evt.Err != nil {
				ch <- StreamEvent{
					Type:  EventError,
					Error: fmt.Sprintf("stream read error: %v", evt.Err),
				}
				return
			}

			var chunk kimiStreamChunk
			if err := json.UnmarshalString(evt.Data, &chunk); err != nil {
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

func (p *KimiProvider) Embed(_ context.Context, _ *EmbeddingRequest) (*EmbeddingResponse, error) {
	return nil, customerrors.ErrEmbeddingNotSupported
}

func (p *KimiProvider) VectorNormalized() bool {
	return false
}
