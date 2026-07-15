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

	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/tools"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
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
	params := p.buildResponseParams(req)

	resp, err := client.Responses.New(ctx, params)
	if err != nil {
		return nil, err
	}

	result := &ChatResponse{
		TotalToken:   resp.Usage.TotalTokens,
		ContextToken: resp.Usage.TotalTokens,
	}

	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			msg := item.AsMessage()
			for _, content := range msg.Content {
				switch content.Type {
				case "output_text":
					text := content.AsOutputText()
					if result.Content != "" {
						result.Content += "\n"
					}
					result.Content += text.Text
				}
			}
		case "function_call":
			fc := item.AsFunctionCall()
			result.ToolCalls = append(result.ToolCalls, models.ToolCall{
				ID:        fc.CallID,
				Name:      fc.Name,
				Arguments: fc.Arguments,
			})
		case "reasoning":
			reasoning := item.AsReasoning()
			block := ReasoningBlock{
				Signature: reasoning.EncryptedContent,
			}
			var summaryParts []string
			for _, s := range reasoning.Summary {
				summaryParts = append(summaryParts, s.Text)
			}
			block.Summary = strings.Join(summaryParts, "\n")
			result.ReasoningBlocks = append(result.ReasoningBlocks, block)
		}
	}

	hydrateChatResponseReasoning(result)

	return result, nil
}

func buildResponseInput(messages []Message) responses.ResponseInputParam {
	var items []responses.ResponseInputItemUnionParam

	for _, msg := range messages {
		switch msg.Role {
		case models.RoleSystem:
			items = append(items, responses.ResponseInputItemParamOfMessage(msg.Content, responses.EasyInputMessageRoleSystem))

		case models.RoleUser:
			items = append(items, responses.ResponseInputItemParamOfMessage(msg.Content, responses.EasyInputMessageRoleUser))

		case models.RoleAssistant:
			if len(msg.ReasoningBlocks) > 0 {
				for _, rb := range msg.ReasoningBlocks {
					summaryParams := []responses.ResponseReasoningItemSummaryParam{
						{Text: rb.Summary},
					}
					reasoningItem := responses.ResponseInputItemParamOfReasoning(rb.Signature, summaryParams)
					if rb.Signature != "" {
						reasoningItem.OfReasoning.EncryptedContent = param.NewOpt(rb.Signature)
					}
					items = append(items, reasoningItem)
				}
			}

			if len(msg.ToolCalls) > 0 {
				if msg.Content != "" {
					items = append(items, responses.ResponseInputItemParamOfMessage(msg.Content, responses.EasyInputMessageRoleAssistant))
				}
				for _, tc := range msg.ToolCalls {
					item := responses.ResponseInputItemParamOfFunctionCall(tc.Arguments, tc.ID, tc.Name)
					items = append(items, item)
				}
			} else {
				items = append(items, responses.ResponseInputItemParamOfMessage(msg.Content, responses.EasyInputMessageRoleAssistant))
			}

		case models.RoleTool:
			if msg.ToolResult != nil {
				items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(msg.ToolResult.CallID, msg.ToolResult.Content))
			}
		}
	}

	return responses.ResponseInputParam(items)
}

func buildResponseTools(defs []tools.ToolDefinition) []responses.ToolUnionParam {
	return lo.Map(defs, func(def tools.ToolDefinition, _ int) responses.ToolUnionParam {
		properties := make(map[string]any)
		for name, prop := range def.Parameters.Properties {
			properties[name] = prop.ToMap()
		}
		tool := responses.ToolParamOfFunction(def.Name, map[string]any{
			"type":       def.Parameters.Type,
			"properties": properties,
			"required":   def.Parameters.Required,
		}, true)
		tool.OfFunction.Description = param.NewOpt(def.Description)
		return tool
	})
}

func validateReasoningEffort(level string) shared.ReasoningEffort {
	switch level {
	case "none":
		return shared.ReasoningEffortNone
	case "minimal":
		return shared.ReasoningEffortMinimal
	case "low":
		return shared.ReasoningEffortLow
	case "medium":
		return shared.ReasoningEffortMedium
	case "high":
		return shared.ReasoningEffortHigh
	case "xhigh":
		return shared.ReasoningEffortXhigh
	default:
		return shared.ReasoningEffortMedium
	}
}

func (p *OpenAIProvider) buildResponseParams(req *ChatRequest) responses.ResponseNewParams {
	input := buildResponseInput(req.Messages)
	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(req.Model),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
		Include: []responses.ResponseIncludable{
			responses.ResponseIncludableReasoningEncryptedContent,
		},
		Store: param.NewOpt(false),
	}

	if len(req.Tools) > 0 {
		params.Tools = buildResponseTools(req.Tools)
	}

	if req.Thinking {
		effort := validateReasoningEffort(req.ThinkingLevel)
		params.Reasoning = shared.ReasoningParam{
			Effort:  effort,
			Summary: shared.ReasoningSummaryAuto,
		}
	}

	if req.ResponseFormat != nil {
		switch req.ResponseFormat.Type {
		case "json_object":
			params.Text = responses.ResponseTextConfigParam{
				Format: responses.ResponseFormatTextConfigUnionParam{
					OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
				},
			}
		case "json_schema":
			if req.ResponseFormat.JSONSchema != nil {
				schema := req.ResponseFormat.JSONSchema
				schemaParam := responses.ResponseFormatTextJSONSchemaConfigParam{
					Name:   schema.Name,
					Schema: schema.Schema,
					Strict: param.NewOpt(schema.Strict),
				}
				if schema.Description != "" {
					schemaParam.Description = param.NewOpt(schema.Description)
				}
				params.Text = responses.ResponseTextConfigParam{
					Format: responses.ResponseFormatTextConfigUnionParam{
						OfJSONSchema: &schemaParam,
					},
				}
			}
		}
	}

	return params
}

func (p *OpenAIProvider) StreamChat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	client := conn.GetOpenAIClient(req.BaseURL, req.APIKey)
	params := p.buildResponseParams(req)

	stream := client.Responses.NewStreaming(ctx, params)

	ch := make(chan StreamEvent, 64)

	safe.GoOnce("openai-stream", func() {
		defer close(ch)
		toolCalls := make(map[string]*models.ToolCall)
		var reasoningBlocks []ReasoningBlock
		var contentBuilder strings.Builder

		for stream.Next() {
			event := stream.Current()

			switch event.Type {
			case "response.output_text.delta":
				ch <- StreamEvent{
					Type:    EventContentDelta,
					Content: event.Delta,
				}
				contentBuilder.WriteString(event.Delta)

			case "response.reasoning_summary_text.delta":
				ch <- StreamEvent{
					Type:      EventReasoningDelta,
					Reasoning: event.Delta,
				}

			case "response.function_call_arguments.delta":
				tc, ok := toolCalls[event.ItemID]
				if ok {
					tc.Arguments += event.Delta
					ch <- StreamEvent{
						Type:     EventToolCallDelta,
						ToolCall: &models.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: event.Delta},
					}
				}

			case "response.output_item.added":
				item := event.Item
				switch item.Type {
				case "function_call":
					fc := item.AsFunctionCall()
					tc := &models.ToolCall{
						ID:   fc.CallID,
						Name: fc.Name,
					}
					toolCalls[fc.ID] = tc
					ch <- StreamEvent{
						Type:     EventToolCallStart,
						ToolCall: &models.ToolCall{ID: fc.CallID, Name: fc.Name},
					}
				}

			case "response.output_item.done":
				item := event.Item
				switch item.Type {
				case "function_call":
					fc := item.AsFunctionCall()
					tc := &models.ToolCall{
						ID:        fc.CallID,
						Name:      fc.Name,
						Arguments: fc.Arguments,
					}
					ch <- StreamEvent{
						Type:     EventToolCallDone,
						ToolCall: tc,
					}
				case "reasoning":
					reasoning := item.AsReasoning()
					block := ReasoningBlock{
						Signature: reasoning.EncryptedContent,
					}
					var summaryParts []string
					for _, s := range reasoning.Summary {
						summaryParts = append(summaryParts, s.Text)
					}
					block.Summary = strings.Join(summaryParts, "\n")
					reasoningBlocks = append(reasoningBlocks, block)
				}

			case "response.completed":
				completed := event.AsResponseCompleted()
				ch <- StreamEvent{
					Type: EventUsage,
					Usage: &StreamUsage{
						TotalTokens:   completed.Response.Usage.TotalTokens,
						ContextTokens: completed.Response.Usage.TotalTokens,
					},
				}
			}
		}

		if err := stream.Err(); err != nil {
			ch <- StreamEvent{
				Type:  EventError,
				Error: fmt.Sprintf("OpenAI streaming error: %v", err),
			}
			return
		}

		finalToolCalls := make([]models.ToolCall, 0, len(toolCalls))
		for _, tc := range toolCalls {
			finalToolCalls = append(finalToolCalls, *tc)
		}

		msg := &Message{
			Role:            models.RoleAssistant,
			Content:         contentBuilder.String(),
			ToolCalls:       finalToolCalls,
			ReasoningBlocks: reasoningBlocks,
		}
		HydrateMessageReasoning(msg)
		ch <- StreamEvent{
			Type:    EventMessageDone,
			Message: msg,
		}
	})

	return ch, nil
}

func (p *OpenAIProvider) Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	client := conn.GetOpenAIClient(req.BaseURL, req.APIKey)

	params := openai.EmbeddingNewParams{
		Model: req.Model,
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: req.Texts,
		},
	}
	if req.Dimensions > 0 {
		params.Dimensions = param.NewOpt(req.Dimensions)
	}

	resp, err := client.Embeddings.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create embeddings: %w", err)
	}

	result := make([][]float32, len(req.Texts))
	for _, item := range resp.Data {
		if int(item.Index) >= len(result) {
			continue
		}
		result[item.Index] = lo.Map(item.Embedding, func(v float64, _ int) float32 {
			return float32(v)
		})
	}
	return &EmbeddingResponse{Embeddings: result}, nil
}

func (p *OpenAIProvider) VectorNormalized() bool {
	return true
}
