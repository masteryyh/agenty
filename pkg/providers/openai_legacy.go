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

	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	"github.com/openai/openai-go/v3/shared/constant"
	"github.com/samber/lo"
)

type OpenAILegacyProvider struct{}

func NewOpenAILegacyProvider() *OpenAILegacyProvider {
	return &OpenAILegacyProvider{}
}

func (p *OpenAILegacyProvider) Name() string {
	return "openai-legacy"
}

func (p *OpenAILegacyProvider) buildChatParams(req *ChatRequest) openai.ChatCompletionNewParams {
	messages := buildOpenAIMessages(req.Messages)
	params := openai.ChatCompletionNewParams{
		Model:    req.Model,
		Messages: messages,
	}

	if len(req.Tools) > 0 {
		params.Tools = buildOpenAITools(req.Tools)
	}

	if req.ResponseFormat != nil {
		switch req.ResponseFormat.Type {
		case "json_object":
			jsonFormat := shared.NewResponseFormatJSONObjectParam()
			params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
				OfJSONObject: &jsonFormat,
			}
		case "json_schema":
			if req.ResponseFormat.JSONSchema != nil {
				schema := req.ResponseFormat.JSONSchema
				jsonSchemaParam := shared.ResponseFormatJSONSchemaParam{
					JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
						Name:   schema.Name,
						Schema: schema.Schema,
						Strict: param.NewOpt(schema.Strict),
					},
					Type: constant.JSONSchema("json_schema"),
				}
				if schema.Description != "" {
					jsonSchemaParam.JSONSchema.Description = param.NewOpt(schema.Description)
				}
				params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
					OfJSONSchema: &jsonSchemaParam,
				}
			}
		}
	}

	return params
}

func (p *OpenAILegacyProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	client := conn.GetOpenAIClient(req.BaseURL, req.APIKey)
	params := p.buildChatParams(req)

	resp, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, err
	}

	result := &ChatResponse{
		TotalToken: resp.Usage.TotalTokens,
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		result.Content = choice.Message.Content

		if len(choice.Message.ToolCalls) > 0 {
			result.ToolCalls = lo.Map(choice.Message.ToolCalls, func(tc openai.ChatCompletionMessageToolCallUnion, _ int) models.ToolCall {
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

func buildOpenAIMessages(messages []Message) []openai.ChatCompletionMessageParamUnion {
	return lo.FilterMap(messages, func(msg Message, _ int) (openai.ChatCompletionMessageParamUnion, bool) {
		switch msg.Role {
		case models.RoleUser:
			return openai.UserMessage(msg.Content), true
		case models.RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				assistantMsg := openai.AssistantMessage(msg.Content)
				assistantMsg.OfAssistant.ToolCalls = lo.Map(msg.ToolCalls, func(tc models.ToolCall, _ int) openai.ChatCompletionMessageToolCallUnionParam {
					return openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: tc.ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Name,
								Arguments: string(tc.Arguments),
							},
						},
					}
				})
				return assistantMsg, true
			}
			return openai.AssistantMessage(msg.Content), true
		case models.RoleTool:
			if msg.ToolResult != nil {
				return openai.ToolMessage(msg.ToolResult.Content, msg.ToolResult.CallID), true
			}
			return openai.ChatCompletionMessageParamUnion{}, false
		case models.RoleSystem:
			return openai.SystemMessage(msg.Content), true
		default:
			return openai.ChatCompletionMessageParamUnion{}, false
		}
	})
}

func buildOpenAITools(defs []tools.ToolDefinition) []openai.ChatCompletionToolUnionParam {
	return lo.Map(defs, func(def tools.ToolDefinition, _ int) openai.ChatCompletionToolUnionParam {
		properties := make(map[string]shared.FunctionParameters)
		for name, prop := range def.Parameters.Properties {
			properties[name] = shared.FunctionParameters(prop.ToMap())
		}
		return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        def.Name,
			Description: param.NewOpt(def.Description),
			Parameters: shared.FunctionParameters{
				"type":       def.Parameters.Type,
				"properties": properties,
				"required":   def.Parameters.Required,
			},
		})
	})
}

func (p *OpenAILegacyProvider) StreamChat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	client := conn.GetOpenAIClient(req.BaseURL, req.APIKey)
	params := p.buildChatParams(req)

	stream := client.Chat.Completions.NewStreaming(ctx, params)

	ch := make(chan StreamEvent, 64)

	safe.GoOnce("openai-legacy-stream", func() {
		defer close(ch)
		type toolCallAccum struct {
			id          string
			name        string
			argsBuilder strings.Builder
		}
		var contentBuilder strings.Builder
		tcMap := make(map[int64]*toolCallAccum)
		var totalTokens int64

		for stream.Next() {
			chunk := stream.Current()

			if chunk.Usage.TotalTokens > 0 {
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

			for _, tc := range delta.ToolCalls {
				acc, ok := tcMap[tc.Index]
				if !ok {
					acc = &toolCallAccum{
						id:   tc.ID,
						name: tc.Function.Name,
					}
					tcMap[tc.Index] = acc
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

		if err := stream.Err(); err != nil {
			ch <- StreamEvent{
				Type:  EventError,
				Error: fmt.Sprintf("OpenAI Legacy streaming error: %v", err),
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
		for _, acc := range tcMap {
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
				Role:      models.RoleAssistant,
				Content:   contentBuilder.String(),
				ToolCalls: toolCalls,
			},
		}
	})

	return ch, nil
}

func (p *OpenAILegacyProvider) Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	return nil, customerrors.ErrEmbeddingNotSupported
}

func (p *OpenAILegacyProvider) VectorNormalized() bool {
	return false
}
