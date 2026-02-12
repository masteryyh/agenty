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

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/samber/lo"
)

type AnthropicProvider struct{}

func NewAnthropicProvider() *AnthropicProvider {
	return &AnthropicProvider{}
}

func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

func (p *AnthropicProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	client := conn.GetAnthropicClient(req.BaseURL, req.APIKey)

	systemPrompts, messages := buildAnthropicMessages(req.Messages)
	params := anthropic.MessageNewParams{
		Model:    anthropic.Model(req.Model),
		System:   systemPrompts,
		Messages: messages,
	}

	if req.MaxTokens > 0 {
		params.MaxTokens = req.MaxTokens
	}

	if len(req.Tools) > 0 {
		params.Tools = buildAnthropicTools(req.Tools)
	}

	resp, err := client.Messages.New(ctx, params)
	if err != nil {
		return nil, err
	}

	result := &ChatResponse{
		TotalToken: resp.Usage.InputTokens + resp.Usage.OutputTokens,
	}

	var textParts []string
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, models.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(block.Input),
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

func buildAnthropicMessages(messages []Message) ([]anthropic.TextBlockParam, []anthropic.MessageParam) {
	systemMessages := make([]anthropic.TextBlockParam, 0)
	params := lo.FilterMap(messages, func(msg Message, _ int) (anthropic.MessageParam, bool) {
		switch msg.Role {
		case models.RoleSystem:
			systemMessages = append(systemMessages, *anthropic.NewTextBlock(msg.Content).OfText)
			return anthropic.MessageParam{}, false
		case models.RoleUser:
			return anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)), true
		case models.RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				var blocks []anthropic.ContentBlockParamUnion
				if msg.Content != "" {
					blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
				}
				for _, tc := range msg.ToolCalls {
					var input any
					if err := sonic.Unmarshal([]byte(tc.Arguments), &input); err != nil {
						input = map[string]any{}
					}
					blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, input, tc.Name))
				}
				return anthropic.NewAssistantMessage(blocks...), true
			}
			return anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content)), true
		case models.RoleTool:
			if msg.ToolResult != nil {
				return anthropic.NewUserMessage(
					anthropic.NewToolResultBlock(msg.ToolResult.CallID, msg.ToolResult.Content, msg.ToolResult.IsError),
				), true
			}
			return anthropic.MessageParam{}, false
		default:
			return anthropic.MessageParam{}, false
		}
	})
	return systemMessages, params
}

func buildAnthropicTools(defs []tools.ToolDefinition) []anthropic.ToolUnionParam {
	return lo.Map(defs, func(def tools.ToolDefinition, _ int) anthropic.ToolUnionParam {
		properties := make(map[string]any)
		for name, prop := range def.Parameters.Properties {
			properties[name] = map[string]any{
				"type":        prop.Type,
				"description": prop.Description,
			}
		}
		tool := anthropic.ToolUnionParamOfTool(
			anthropic.ToolInputSchemaParam{
				Properties: properties,
				Required:   def.Parameters.Required,
			},
			def.Name,
		)
		tool.OfTool.Description = param.NewOpt(def.Description)
		return tool
	})
}
