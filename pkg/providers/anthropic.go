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

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/safe"
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
	params := p.buildMessageParams(req)

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
		case "thinking":
			result.ReasoningBlocks = append(result.ReasoningBlocks, ReasoningBlock{
				Summary:   block.Thinking,
				Signature: block.Signature,
			})
		case "redacted_thinking":
			result.ReasoningBlocks = append(result.ReasoningBlocks, ReasoningBlock{
				Summary:   "",
				Signature: block.Data,
				Redacted:  true,
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
			blocks := []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(msg.Content)}
			if len(msg.ReasoningBlocks) > 0 {
				thinkingBlocks := lo.Map(msg.ReasoningBlocks, func(rb ReasoningBlock, _ int) anthropic.ContentBlockParamUnion {
					if rb.Redacted {
						return anthropic.NewRedactedThinkingBlock(rb.Signature)
					}
					return anthropic.NewThinkingBlock(rb.Signature, rb.Summary)
				})
				blocks = append(thinkingBlocks, blocks...)
				return anthropic.NewUserMessage(blocks...), true
			}
			return anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)), true

		case models.RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				var blocks []anthropic.ContentBlockParamUnion
				if len(msg.ReasoningBlocks) > 0 {
					thinkingBlocks := lo.Map(msg.ReasoningBlocks, func(rb ReasoningBlock, _ int) anthropic.ContentBlockParamUnion {
						if rb.Redacted {
							return anthropic.NewRedactedThinkingBlock(rb.Signature)
						}
						return anthropic.NewThinkingBlock(rb.Signature, rb.Summary)
					})
					blocks = append(thinkingBlocks, blocks...)
				}
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
				blocks := []anthropic.ContentBlockParamUnion{anthropic.NewToolResultBlock(msg.ToolResult.CallID, msg.ToolResult.Content, msg.ToolResult.IsError)}
				return anthropic.NewUserMessage(blocks...), true
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
			properties[name] = prop.ToMap()
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

func (p *AnthropicProvider) buildMessageParams(req *ChatRequest) anthropic.MessageNewParams {
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

	if req.Thinking {
		if req.AnthropicAdaptiveThinking {
			params.Thinking = anthropic.ThinkingConfigParamUnion{
				OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
			}
		} else {
			params.Thinking = anthropic.ThinkingConfigParamOfEnabled(31999)
		}
	}

	return params
}

func (p *AnthropicProvider) StreamChat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	client := conn.GetAnthropicClient(req.BaseURL, req.APIKey)
	params := p.buildMessageParams(req)

	stream := client.Messages.NewStreaming(ctx, params)

	ch := make(chan StreamEvent, 64)

	safe.GoOnce("anthropic-stream", func() {
		defer close(ch)
		var currentThinkingContent strings.Builder
		var currentSignature string
		var toolCalls []models.ToolCall
		var textParts []string
		var currentToolID, currentToolName string
		var currentToolArgs strings.Builder
		var totalTokens int64
		var reasoningBlocks []ReasoningBlock

		type blockInfo struct {
			blockType string
		}
		blockMap := make(map[int64]*blockInfo)

		for stream.Next() {
			event := stream.Current()

			switch variant := event.AsAny().(type) {
			case anthropic.ContentBlockStartEvent:
				bi := &blockInfo{blockType: variant.ContentBlock.Type}
				blockMap[variant.Index] = bi

				switch variant.ContentBlock.Type {
				case "tool_use":
					currentToolID = variant.ContentBlock.ID
					currentToolName = variant.ContentBlock.Name
					currentToolArgs.Reset()
					ch <- StreamEvent{
						Type:     EventToolCallStart,
						ToolCall: &models.ToolCall{ID: currentToolID, Name: currentToolName},
					}
				case "thinking":
					currentThinkingContent.Reset()
					currentSignature = ""
				}

			case anthropic.ContentBlockDeltaEvent:
				switch deltaVariant := variant.Delta.AsAny().(type) {
				case anthropic.TextDelta:
					ch <- StreamEvent{
						Type:    EventContentDelta,
						Content: deltaVariant.Text,
					}
					textParts = append(textParts, deltaVariant.Text)

				case anthropic.ThinkingDelta:
					ch <- StreamEvent{
						Type:      EventReasoningDelta,
						Reasoning: deltaVariant.Thinking,
					}
					currentThinkingContent.WriteString(deltaVariant.Thinking)

				case anthropic.InputJSONDelta:
					currentToolArgs.WriteString(deltaVariant.PartialJSON)
					ch <- StreamEvent{
						Type:     EventToolCallDelta,
						ToolCall: &models.ToolCall{ID: currentToolID, Name: currentToolName, Arguments: deltaVariant.PartialJSON},
					}

				case anthropic.SignatureDelta:
					currentSignature = deltaVariant.Signature
				}

			case anthropic.ContentBlockStopEvent:
				bi, ok := blockMap[variant.Index]
				if !ok {
					continue
				}
				switch bi.blockType {
				case "tool_use":
					tc := models.ToolCall{
						ID:        currentToolID,
						Name:      currentToolName,
						Arguments: currentToolArgs.String(),
					}
					toolCalls = append(toolCalls, tc)
					ch <- StreamEvent{
						Type:     EventToolCallDone,
						ToolCall: &tc,
					}
				case "thinking":
					block := ReasoningBlock{
						Summary:   currentThinkingContent.String(),
						Signature: currentSignature,
					}
					reasoningBlocks = append(reasoningBlocks, block)
				case "redacted_thinking":
					block := ReasoningBlock{
						Redacted:  true,
						Signature: currentSignature,
					}
					reasoningBlocks = append(reasoningBlocks, block)
				}

			case anthropic.MessageDeltaEvent:
				if variant.Usage.OutputTokens > 0 {
					totalTokens += variant.Usage.OutputTokens
				}

			case anthropic.MessageStartEvent:
				if variant.Message.Usage.InputTokens > 0 {
					totalTokens += variant.Message.Usage.InputTokens
				}
			}
		}

		if err := stream.Err(); err != nil {
			ch <- StreamEvent{
				Type:  EventError,
				Error: fmt.Sprintf("Anthropic streaming error: %v", err),
			}
			return
		}

		ch <- StreamEvent{
			Type:  EventUsage,
			Usage: &StreamUsage{TotalTokens: totalTokens},
		}

		var content string
		if len(textParts) > 0 {
			content = strings.Join(textParts, "")
		}

		msg := &Message{
			Role:            models.RoleAssistant,
			Content:         content,
			ToolCalls:       toolCalls,
			ReasoningBlocks: reasoningBlocks,
		}
		if len(reasoningBlocks) > 0 {
			var summaryBuilder strings.Builder
			for _, block := range reasoningBlocks {
				if !block.Redacted {
					summaryBuilder.WriteString(block.Summary)
					summaryBuilder.WriteString("\n")
				}
			}
			msg.ReasoningContent = summaryBuilder.String()
		}
		ch <- StreamEvent{
			Type:    EventMessageDone,
			Message: msg,
		}
	})

	return ch, nil
}

func (p *AnthropicProvider) Embed(_ context.Context, _ *EmbeddingRequest) (*EmbeddingResponse, error) {
	return nil, customerrors.ErrEmbeddingNotSupported
}

func (p *AnthropicProvider) VectorNormalized() bool {
	return false
}
