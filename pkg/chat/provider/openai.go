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
	"context"

	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	"github.com/samber/lo"
)

type OpenAIProvider struct{}

func NewOpenAIProvider() *OpenAIProvider {
	return &OpenAIProvider{}
}

func (p *OpenAIProvider) Name() string {
	return "openai"
}

func (p *OpenAIProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	client := conn.GetOpenAIClient(req.BaseURL, req.APIKey)

	messages := buildOpenAIMessages(req.Messages)

	params := openai.ChatCompletionNewParams{
		Model:    req.Model,
		Messages: messages,
	}

	if len(req.Tools) > 0 {
		params.Tools = buildOpenAITools(req.Tools)
	}

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
		case RoleUser:
			return openai.UserMessage(msg.Content), true
		case RoleAssistant:
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
		case RoleTool:
			if msg.ToolResult != nil {
				return openai.ToolMessage(msg.ToolResult.Content, msg.ToolResult.CallID), true
			}
			return openai.ChatCompletionMessageParamUnion{}, false
		case RoleSystem:
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
			properties[name] = shared.FunctionParameters{
				"type":        prop.Type,
				"description": prop.Description,
			}
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
