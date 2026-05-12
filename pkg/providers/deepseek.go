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
	"unicode"
	"unicode/utf8"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/tools"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"github.com/samber/lo"
)

const (
	deepSeekDefaultBaseURL = "https://api.deepseek.com"
	deepSeekToolTypeFunc   = "function"
)

type DeepSeekProvider struct{}

func NewDeepSeekProvider() *DeepSeekProvider {
	return &DeepSeekProvider{}
}

func (p *DeepSeekProvider) Name() string {
	return "deepseek"
}

type deepSeekRequest struct {
	Model           string                  `json:"model"`
	Messages        []deepSeekMessage       `json:"messages"`
	Thinking        *deepSeekThinking       `json:"thinking,omitempty"`
	ReasoningEffort string                  `json:"reasoning_effort,omitempty"`
	Tools           []deepSeekTool          `json:"tools,omitempty"`
	ResponseFormat  *deepSeekResponseFormat `json:"response_format,omitempty"`
	Stream          bool                    `json:"stream,omitempty"`
	StreamOptions   *deepSeekStreamOptions  `json:"stream_options,omitempty"`
}

type deepSeekThinking struct {
	Type string `json:"type"`
}

type deepSeekResponseFormat struct {
	Type string `json:"type"`
}

type deepSeekStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type deepSeekMessage struct {
	Role             models.MessageRole `json:"role"`
	Content          string             `json:"content"`
	ToolCalls        []deepSeekToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string             `json:"tool_call_id,omitempty"`
	ReasoningContent string             `json:"reasoning_content,omitempty"`
}

type deepSeekToolCall struct {
	Index    int                  `json:"index,omitempty"`
	ID       string               `json:"id,omitempty"`
	Type     string               `json:"type,omitempty"`
	Function deepSeekToolCallFunc `json:"function"`
}

type deepSeekToolCallFunc struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type deepSeekTool struct {
	Type     string              `json:"type"`
	Function deepSeekToolFuncDef `json:"function"`
}

type deepSeekToolFuncDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type deepSeekResponse struct {
	Choices []deepSeekChoice `json:"choices"`
	Usage   deepSeekUsage    `json:"usage"`
	Error   *deepSeekError   `json:"error,omitempty"`
}

type deepSeekChoice struct {
	Message deepSeekMessage `json:"message"`
}

type deepSeekUsage struct {
	TotalTokens int64 `json:"total_tokens"`
}

type deepSeekError struct {
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
}

type deepSeekStreamChunk struct {
	Choices []deepSeekStreamChoice `json:"choices"`
	Usage   *deepSeekUsage         `json:"usage,omitempty"`
	Error   *deepSeekError         `json:"error,omitempty"`
}

type deepSeekStreamChoice struct {
	Delta deepSeekStreamDelta `json:"delta"`
}

type deepSeekStreamDelta struct {
	Content          string             `json:"content,omitempty"`
	ReasoningContent string             `json:"reasoning_content,omitempty"`
	ToolCalls        []deepSeekToolCall `json:"tool_calls,omitempty"`
	Role             string             `json:"role,omitempty"`
}

func deepSeekSanitizeText(s string) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	return strings.Map(func(r rune) rune {
		if r == utf8.RuneError || isDeepSeekInvalidCodePoint(r) {
			return '\uFFFD'
		}
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return ' '
		}
		return r
	}, s)
}

func isDeepSeekInvalidCodePoint(r rune) bool {
	if r < 0 || r > utf8.MaxRune {
		return true
	}
	if r >= 0xD800 && r <= 0xDFFF {
		return true
	}
	if r >= 0xFDD0 && r <= 0xFDEF {
		return true
	}
	return r&0xFFFE == 0xFFFE
}

func deepSeekSanitizeAny(v any) any {
	switch value := v.(type) {
	case string:
		return deepSeekSanitizeText(value)
	case map[string]any:
		result := make(map[string]any, len(value))
		for k, item := range value {
			result[deepSeekSanitizeText(k)] = deepSeekSanitizeAny(item)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for i, item := range value {
			result[i] = deepSeekSanitizeAny(item)
		}
		return result
	case []string:
		result := make([]string, len(value))
		for i, item := range value {
			result[i] = deepSeekSanitizeText(item)
		}
		return result
	default:
		return v
	}
}

func buildDeepSeekMessages(messages []Message, includeReasoning bool) []deepSeekMessage {
	return lo.FilterMap(messages, func(msg Message, _ int) (deepSeekMessage, bool) {
		switch msg.Role {
		case models.RoleUser:
			return deepSeekMessage{
				Role:    models.RoleUser,
				Content: deepSeekSanitizeText(msg.Content),
			}, true
		case models.RoleAssistant:
			dm := deepSeekMessage{
				Role:    models.RoleAssistant,
				Content: deepSeekSanitizeText(msg.Content),
			}
			if includeReasoning {
				dm.ReasoningContent = deepSeekSanitizeText(msg.ReasoningContent)
			}
			if len(msg.ToolCalls) > 0 {
				dm.ToolCalls = lo.Map(msg.ToolCalls, func(tc models.ToolCall, _ int) deepSeekToolCall {
					return deepSeekToolCall{
						ID:   deepSeekSanitizeText(tc.ID),
						Type: deepSeekToolTypeFunc,
						Function: deepSeekToolCallFunc{
							Name:      deepSeekSanitizeText(tc.Name),
							Arguments: deepSeekSanitizeText(string(tc.Arguments)),
						},
					}
				})
			}
			return dm, true
		case models.RoleTool:
			if msg.ToolResult != nil {
				return deepSeekMessage{
					Role:       models.RoleTool,
					Content:    deepSeekSanitizeText(msg.ToolResult.Content),
					ToolCallID: deepSeekSanitizeText(msg.ToolResult.CallID),
				}, true
			}
			return deepSeekMessage{}, false
		case models.RoleSystem:
			return deepSeekMessage{
				Role:    models.RoleSystem,
				Content: deepSeekSanitizeText(msg.Content),
			}, true
		default:
			return deepSeekMessage{}, false
		}
	})
}

func buildDeepSeekTools(defs []tools.ToolDefinition) []deepSeekTool {
	return lo.Map(defs, func(def tools.ToolDefinition, _ int) deepSeekTool {
		properties := make(map[string]any)
		for name, prop := range def.Parameters.Properties {
			properties[deepSeekSanitizeText(name)] = deepSeekSanitizeAny(prop.ToMap())
		}
		return deepSeekTool{
			Type: deepSeekToolTypeFunc,
			Function: deepSeekToolFuncDef{
				Name:        deepSeekSanitizeText(def.Name),
				Description: deepSeekSanitizeText(def.Description),
				Parameters: map[string]any{
					"type":       def.Parameters.Type,
					"properties": properties,
					"required":   deepSeekSanitizeAny(def.Parameters.Required),
				},
			},
		}
	})
}

func deepSeekReasoningEffort(level string) string {
	switch level {
	case "max", "xhigh":
		return "max"
	default:
		return "high"
	}
}

func (p *DeepSeekProvider) buildRequest(req *ChatRequest, stream bool) deepSeekRequest {
	apiReq := deepSeekRequest{
		Model:    deepSeekSanitizeText(req.Model),
		Messages: buildDeepSeekMessages(req.Messages, req.Thinking),
		Thinking: &deepSeekThinking{Type: "disabled"},
		Stream:   stream,
	}

	if req.Thinking {
		apiReq.Thinking = &deepSeekThinking{Type: "enabled"}
		apiReq.ReasoningEffort = deepSeekReasoningEffort(req.ThinkingLevel)
	}

	if stream {
		apiReq.StreamOptions = &deepSeekStreamOptions{IncludeUsage: true}
	}

	if len(req.Tools) > 0 {
		apiReq.Tools = buildDeepSeekTools(req.Tools)
	}

	if req.ResponseFormat != nil && req.ResponseFormat.Type == "json_object" {
		apiReq.ResponseFormat = &deepSeekResponseFormat{Type: "json_object"}
	}

	return apiReq
}

func (p *DeepSeekProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = deepSeekDefaultBaseURL
	}

	apiResp, err := conn.Post[deepSeekResponse](ctx, conn.HTTPRequest{
		URL:     baseURL + "/chat/completions",
		Headers: map[string]string{"Authorization": "Bearer " + req.APIKey},
		Body:    p.buildRequest(req, false),
	})
	if err != nil {
		return nil, err
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("API error: %s", apiResp.Error.Message)
	}

	result := &ChatResponse{
		TotalToken:   apiResp.Usage.TotalTokens,
		ContextToken: apiResp.Usage.TotalTokens,
	}

	if len(apiResp.Choices) > 0 {
		choice := apiResp.Choices[0]
		result.Content = choice.Message.Content
		result.ReasoningContent = choice.Message.ReasoningContent
		if len(choice.Message.ToolCalls) > 0 {
			result.ToolCalls = lo.Map(choice.Message.ToolCalls, func(tc deepSeekToolCall, _ int) models.ToolCall {
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

func (p *DeepSeekProvider) StreamChat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = deepSeekDefaultBaseURL
	}

	lines, err := conn.PostSSE(ctx, conn.HTTPRequest{
		URL:     baseURL + "/chat/completions",
		Headers: map[string]string{"Authorization": "Bearer " + req.APIKey},
		Body:    p.buildRequest(req, true),
	})
	if err != nil {
		return nil, err
	}

	ch := make(chan StreamEvent, 64)

	safe.GoOnce("deepseek-stream", func() {
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

			var chunk deepSeekStreamChunk
			if err := json.UnmarshalString(evt.Data, &chunk); err != nil {
				continue
			}
			if chunk.Error != nil {
				ch <- StreamEvent{
					Type:  EventError,
					Error: chunk.Error.Message,
				}
				return
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

			for _, tc := range delta.ToolCalls {
				key := tc.Index
				acc, ok := tcMap[key]
				if !ok {
					acc = &toolCallAccum{
						id:   tc.ID,
						name: tc.Function.Name,
					}
					tcMap[key] = acc
					tcKeys = append(tcKeys, key)
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
				Usage: &StreamUsage{TotalTokens: totalTokens, ContextTokens: totalTokens},
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

		ch <- StreamEvent{
			Type: EventMessageDone,
			Message: &Message{
				Role:             models.RoleAssistant,
				Content:          contentBuilder.String(),
				ToolCalls:        toolCalls,
				ReasoningContent: reasoningBuilder.String(),
			},
		}
	})

	return ch, nil
}

func (p *DeepSeekProvider) Embed(_ context.Context, _ *EmbeddingRequest) (*EmbeddingResponse, error) {
	return nil, customerrors.ErrEmbeddingNotSupported
}

func (p *DeepSeekProvider) VectorNormalized() bool {
	return false
}
