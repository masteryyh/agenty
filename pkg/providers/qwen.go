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
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/samber/lo"
)

type QwenProvider struct{}

func NewQwenProvider() *QwenProvider {
	return &QwenProvider{}
}

func (p *QwenProvider) Name() string {
	return "qwen"
}

func buildQwenResponseInput(messages []Message) responses.ResponseInputParam {
	var items []responses.ResponseInputItemUnionParam

	for _, msg := range messages {
		switch msg.Role {
		case models.RoleSystem:
			items = append(items, responses.ResponseInputItemParamOfMessage(msg.Content, responses.EasyInputMessageRoleSystem))

		case models.RoleUser:
			items = append(items, responses.ResponseInputItemParamOfMessage(msg.Content, responses.EasyInputMessageRoleUser))

		case models.RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				if msg.Content != "" {
					items = append(items, responses.ResponseInputItemParamOfMessage(msg.Content, responses.EasyInputMessageRoleAssistant))
				}
				for _, tc := range msg.ToolCalls {
					items = append(items, responses.ResponseInputItemParamOfFunctionCall(tc.Arguments, tc.ID, tc.Name))
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

func (p *QwenProvider) buildResponseParams(req *ChatRequest) responses.ResponseNewParams {
	input := buildQwenResponseInput(req.Messages)
	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(req.Model),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
		Store: param.NewOpt(false),
	}

	if len(req.Tools) > 0 {
		params.Tools = buildResponseTools(req.Tools)
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

func (p *QwenProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	client := conn.GetOpenAIClient(req.BaseURL, req.APIKey)
	params := p.buildResponseParams(req)

	opts := make([]option.RequestOption, 0)
	if req.Thinking {
		opts = append(opts, option.WithJSONSet("enable_thinking", true))
	}

	resp, err := client.Responses.New(ctx, params, opts...)
	if err != nil {
		return nil, err
	}

	result := &ChatResponse{
		TotalToken: resp.Usage.TotalTokens,
	}

	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			msg := item.AsMessage()
			for _, content := range msg.Content {
				if content.Type == "output_text" {
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
			block := ReasoningBlock{}
			var summaryParts []string
			for _, s := range reasoning.Summary {
				summaryParts = append(summaryParts, s.Text)
			}
			block.Summary = strings.Join(summaryParts, "\n")
			result.ReasoningBlocks = append(result.ReasoningBlocks, block)
		}
	}

	if len(result.ReasoningBlocks) > 0 {
		var sb strings.Builder
		for _, block := range result.ReasoningBlocks {
			sb.WriteString(block.Summary)
			sb.WriteString("\n")
		}
		result.ReasoningContent = sb.String()
	}

	return result, nil
}

func (p *QwenProvider) StreamChat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	client := conn.GetOpenAIClient(req.BaseURL, req.APIKey)
	params := p.buildResponseParams(req)

	opts := make([]option.RequestOption, 0)
	if req.Thinking {
		opts = append(opts, option.WithJSONSet("enable_thinking", true))
	}

	stream := client.Responses.NewStreaming(ctx, params, opts...)

	ch := make(chan StreamEvent, 64)

	safe.GoOnce("qwen-stream", func() {
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
				if item.Type == "function_call" {
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
					block := ReasoningBlock{}
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
						TotalTokens: completed.Response.Usage.TotalTokens,
					},
				}
			}
		}

		if err := stream.Err(); err != nil {
			ch <- StreamEvent{
				Type:  EventError,
				Error: fmt.Sprintf("Qwen streaming error: %v", err),
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
		if len(reasoningBlocks) > 0 {
			var sb strings.Builder
			for _, block := range reasoningBlocks {
				sb.WriteString(block.Summary)
				sb.WriteString("\n")
			}
			msg.ReasoningContent = sb.String()
		}
		ch <- StreamEvent{
			Type:    EventMessageDone,
			Message: msg,
		}
	})

	return ch, nil
}

type bailianContentItem struct {
	Text string `json:"text"`
}

type bailianEmbeddingInput struct {
	Contents []bailianContentItem `json:"contents"`
}

type bailianEmbeddingParameters struct {
	Dimension int64 `json:"dimension,omitempty"`
}

type bailianEmbeddingRequest struct {
	Model      string                     `json:"model"`
	Input      bailianEmbeddingInput      `json:"input"`
	Parameters bailianEmbeddingParameters `json:"parameters,omitempty"`
}

type bailianEmbeddingItem struct {
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
	Type      string    `json:"type"`
}

type bailianEmbeddingOutput struct {
	Embeddings []bailianEmbeddingItem `json:"embeddings"`
}

type bailianEmbeddingResponse struct {
	Output    bailianEmbeddingOutput `json:"output"`
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	RequestID string                 `json:"request_id"`
}

func (p *QwenProvider) Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	contents := make([]bailianContentItem, len(req.Texts))
	for i, text := range req.Texts {
		contents[i] = bailianContentItem{Text: text}
	}

	embReq := bailianEmbeddingRequest{
		Model: req.Model,
		Input: bailianEmbeddingInput{Contents: contents},
	}
	if req.Dimensions > 0 {
		embReq.Parameters = bailianEmbeddingParameters{Dimension: req.Dimensions}
	}

	embResp, err := conn.Post[bailianEmbeddingResponse](ctx, conn.HTTPRequest{
		URL:     req.BaseURL,
		Headers: map[string]string{"Authorization": "Bearer " + req.APIKey},
		Body:    embReq,
	})
	if err != nil {
		return nil, err
	}

	if embResp.Code != "" {
		return nil, fmt.Errorf("bailian embedding error %s: %s", embResp.Code, embResp.Message)
	}

	result := make([][]float32, len(req.Texts))
	for _, item := range embResp.Output.Embeddings {
		if item.Index < 0 || item.Index >= len(result) {
			continue
		}
		result[item.Index] = lo.Map(item.Embedding, func(v float64, _ int) float32 {
			return float32(v)
		})
	}
	return &EmbeddingResponse{Embeddings: result}, nil
}

func (p *QwenProvider) VectorNormalized() bool {
	return false
}
