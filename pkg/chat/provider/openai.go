package provider

import (
	"context"
	"encoding/json"

	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
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
			result.ToolCalls = make([]tools.ToolCall, len(choice.Message.ToolCalls))
			for i, tc := range choice.Message.ToolCalls {
				result.ToolCalls[i] = tools.ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: json.RawMessage(tc.Function.Arguments),
				}
			}
		}
	}

	return result, nil
}

func buildOpenAIMessages(messages []Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			result = append(result, openai.UserMessage(msg.Content))
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				assistantMsg := openai.AssistantMessage(msg.Content)
				toolCalls := make([]openai.ChatCompletionMessageToolCallUnionParam, len(msg.ToolCalls))
				for i, tc := range msg.ToolCalls {
					toolCalls[i] = openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: tc.ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Name,
								Arguments: string(tc.Arguments),
							},
						},
					}
				}
				assistantMsg.OfAssistant.ToolCalls = toolCalls
				result = append(result, assistantMsg)
			} else {
				result = append(result, openai.AssistantMessage(msg.Content))
			}
		case "tool":
			if msg.ToolResult != nil {
				result = append(result, openai.ToolMessage(msg.ToolResult.Content, msg.ToolResult.CallID))
			}
		case "system":
			result = append(result, openai.SystemMessage(msg.Content))
		}
	}
	return result
}

func buildOpenAITools(defs []tools.ToolDefinition) []openai.ChatCompletionToolUnionParam {
	result := make([]openai.ChatCompletionToolUnionParam, len(defs))
	for i, def := range defs {
		properties := make(map[string]shared.FunctionParameters)
		for name, prop := range def.Parameters.Properties {
			properties[name] = shared.FunctionParameters{
				"type":        prop.Type,
				"description": prop.Description,
			}
		}

		result[i] = openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        def.Name,
			Description: param.NewOpt(def.Description),
			Parameters: shared.FunctionParameters{
				"type":       def.Parameters.Type,
				"properties": properties,
				"required":   def.Parameters.Required,
			},
		})
	}
	return result
}
