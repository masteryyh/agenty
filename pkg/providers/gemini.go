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
	"encoding/base64"
	"fmt"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/tools"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"github.com/samber/lo"
	"google.golang.org/genai"
)

type GeminiProvider struct{}

func NewGeminiProvider() *GeminiProvider {
	return &GeminiProvider{}
}

func (p *GeminiProvider) Name() string {
	return "gemini"
}

func (p *GeminiProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	client, err := conn.GetGeminiClient(ctx, req.BaseURL, req.APIKey)
	if err != nil {
		return nil, err
	}

	contents := buildGeminiContents(req.Messages)
	config := p.buildContentConfig(req)

	resp, err := client.Models.GenerateContent(ctx, req.Model, contents, config)
	if err != nil {
		return nil, err
	}

	result := &ChatResponse{}
	if resp.UsageMetadata != nil {
		result.TotalToken = int64(resp.UsageMetadata.TotalTokenCount)
	}

	var reasoningBuilder strings.Builder
	var thoughtSignature string
	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		for _, part := range resp.Candidates[0].Content.Parts {
			if part.Text != "" {
				if part.Thought {
					if reasoningBuilder.Len() > 0 {
						reasoningBuilder.WriteString("\n")
					}
					reasoningBuilder.WriteString(part.Text)
				} else {
					if result.Content != "" {
						result.Content += "\n"
					}
					result.Content += part.Text
				}
			}

			if part.FunctionCall != nil {
				argsJSON, err := json.MarshalString(part.FunctionCall.Args)
				if err != nil {
					argsJSON = "{}"
				}
				id := part.FunctionCall.ID
				if id == "" {
					id = fmt.Sprintf("gemini_call_%s_%s", part.FunctionCall.Name, strings.ReplaceAll(uuid.NewString(), "-", ""))
				}
				result.ToolCalls = append(result.ToolCalls, models.ToolCall{
					ID:        id,
					Name:      part.FunctionCall.Name,
					Arguments: argsJSON,
				})
			}
			if len(part.ThoughtSignature) > 0 {
				thoughtSignature = base64.StdEncoding.EncodeToString(part.ThoughtSignature)
			}
		}
	}
	if reasoningBuilder.Len() > 0 || thoughtSignature != "" {
		result.ReasoningBlocks = append(result.ReasoningBlocks, ReasoningBlock{
			Summary:   reasoningBuilder.String(),
			Signature: thoughtSignature,
		})
	}
	hydrateChatResponseReasoning(result)

	return result, nil
}

func buildGeminiContents(messages []Message) []*genai.Content {
	return lo.FilterMap(messages, func(msg Message, _ int) (*genai.Content, bool) {
		switch msg.Role {
		case models.RoleUser:
			return genai.NewContentFromText(msg.Content, genai.RoleUser), true

		case models.RoleAssistant:
			c := &genai.Content{Role: genai.RoleModel}

			var signature []byte
			if len(msg.ReasoningBlocks) > 0 {
				thinkingParts := buildGeminiThoughtChain(msg.ReasoningBlocks)
				c.Parts = append(c.Parts, thinkingParts...)

				for _, reasoningBlock := range msg.ReasoningBlocks {
					if reasoningBlock.Signature != "" {
						sigBytes, err := base64.StdEncoding.DecodeString(reasoningBlock.Signature)
						if err == nil {
							signature = sigBytes
							break
						}
					}
				}
			}

			if msg.Content != "" {
				part := genai.NewPartFromText(msg.Content)
				if len(signature) > 0 {
					part.ThoughtSignature = signature
				}
				c.Parts = append(c.Parts, part)
			}

			tcBlocks := make([]*genai.Part, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				var args map[string]any
				if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
					args = map[string]any{}
				}

				part := genai.NewPartFromFunctionCall(tc.Name, args)
				tcBlocks = append(tcBlocks, part)
			}
			if len(tcBlocks) > 0 {
				tcBlocks[0].ThoughtSignature = signature
			}

			c.Parts = append(c.Parts, tcBlocks...)
			return c, true

		case models.RoleTool:
			if msg.ToolResult != nil {
				return genai.NewContentFromFunctionResponse(
					msg.ToolResult.Name,
					map[string]any{"result": msg.ToolResult.Content},
					genai.RoleUser,
				), true
			}
			return nil, false

		case models.RoleSystem:
			return nil, false

		default:
			return nil, false
		}
	})
}

func extractGeminiSystemInstruction(messages []Message) string {
	var sb strings.Builder
	for _, msg := range messages {
		if msg.Role != models.RoleSystem || msg.Content == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(msg.Content)
	}
	return sb.String()
}

func buildGeminiTools(defs []tools.ToolDefinition) []*genai.Tool {
	decls := lo.Map(defs, func(def tools.ToolDefinition, _ int) *genai.FunctionDeclaration {
		properties := make(map[string]*genai.Schema)
		for name, prop := range def.Parameters.Properties {
			properties[name] = propToGeminiSchema(prop)
		}
		return &genai.FunctionDeclaration{
			Name:        def.Name,
			Description: def.Description,
			Parameters: &genai.Schema{
				Type:       genai.Type(def.Parameters.Type),
				Properties: properties,
				Required:   def.Parameters.Required,
			},
		}
	})
	return []*genai.Tool{{FunctionDeclarations: decls}}
}

func propToGeminiSchema(prop tools.ParameterProperty) *genai.Schema {
	s := &genai.Schema{
		Type:        genai.Type(prop.Type),
		Description: prop.Description,
	}
	if prop.Items != nil {
		s.Items = propToGeminiSchema(*prop.Items)
	}
	return s
}

func buildGeminiThoughtChain(blocks []ReasoningBlock) []*genai.Part {
	return lo.FilterMap(blocks, func(block ReasoningBlock, _ int) (*genai.Part, bool) {
		if block.Summary != "" {
			return &genai.Part{
				Thought: true,
				Text:    block.Summary,
			}, true
		}
		return nil, false
	})
}

func validateGeminiThinkingLevel(level string) genai.ThinkingLevel {
	switch level {
	case "low":
		return genai.ThinkingLevelLow
	case "medium":
		return genai.ThinkingLevelMedium
	case "high":
		return genai.ThinkingLevelHigh
	default:
		return genai.ThinkingLevelMedium
	}
}

func (p *GeminiProvider) buildContentConfig(req *ChatRequest) *genai.GenerateContentConfig {
	config := &genai.GenerateContentConfig{}
	if systemInstruction := extractGeminiSystemInstruction(req.Messages); systemInstruction != "" {
		config.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: systemInstruction}}}
	}
	if len(req.Tools) > 0 {
		config.Tools = buildGeminiTools(req.Tools)
	}

	if req.ResponseFormat != nil && req.ResponseFormat.Type == "json_object" {
		config.ResponseMIMEType = "application/json"
	}

	if req.Thinking {
		config.ThinkingConfig = &genai.ThinkingConfig{
			IncludeThoughts: true,
			ThinkingLevel:   validateGeminiThinkingLevel(req.ThinkingLevel),
		}
	}

	return config
}

func (p *GeminiProvider) StreamChat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	client, err := conn.GetGeminiClient(ctx, req.BaseURL, req.APIKey)
	if err != nil {
		return nil, err
	}

	contents := buildGeminiContents(req.Messages)
	config := p.buildContentConfig(req)

	ch := make(chan StreamEvent, 64)

	safe.GoOnce("gemini-stream", func() {
		defer close(ch)
		var contentBuilder strings.Builder
		var reasoningBuilder strings.Builder
		var toolCalls []models.ToolCall
		var totalTokens int64
		var thoughtSignature string

		for result, err := range client.Models.GenerateContentStream(ctx, req.Model, contents, config) {
			if err != nil {
				ch <- StreamEvent{
					Type:  EventError,
					Error: fmt.Sprintf("Gemini streaming error: %v", err),
				}
				return
			}

			if result.UsageMetadata != nil {
				totalTokens = int64(result.UsageMetadata.TotalTokenCount)
			}

			if len(result.Candidates) > 0 && result.Candidates[0].Content != nil {
				for _, part := range result.Candidates[0].Content.Parts {
					if part.Text != "" {
						if part.Thought {
							ch <- StreamEvent{
								Type:      EventReasoningDelta,
								Reasoning: part.Text,
							}
							if reasoningBuilder.Len() > 0 {
								reasoningBuilder.WriteString("\n")
							}
							reasoningBuilder.WriteString(part.Text)
						} else {
							ch <- StreamEvent{
								Type:    EventContentDelta,
								Content: part.Text,
							}
							contentBuilder.WriteString(part.Text)
						}
					}

					if part.FunctionCall != nil {
						argsJSON, jsonErr := json.MarshalString(part.FunctionCall.Args)
						if jsonErr != nil {
							argsJSON = "{}"
						}
						id := part.FunctionCall.ID
						if id == "" {
							id = fmt.Sprintf("gemini_call_%s_%s", part.FunctionCall.Name, strings.ReplaceAll(uuid.NewString(), "-", ""))
						}
						tc := models.ToolCall{
							ID:        id,
							Name:      part.FunctionCall.Name,
							Arguments: argsJSON,
						}
						toolCalls = append(toolCalls, tc)
						ch <- StreamEvent{
							Type:     EventToolCallStart,
							ToolCall: &tc,
						}
						ch <- StreamEvent{
							Type:     EventToolCallDone,
							ToolCall: &tc,
						}
					}

					if len(part.ThoughtSignature) > 0 {
						thoughtSignature = base64.StdEncoding.EncodeToString(part.ThoughtSignature)
					}
				}
			}
		}

		ch <- StreamEvent{
			Type:  EventUsage,
			Usage: &StreamUsage{TotalTokens: totalTokens},
		}

		msg := &Message{
			Role:      models.RoleAssistant,
			Content:   contentBuilder.String(),
			ToolCalls: toolCalls,
		}
		if reasoningBuilder.Len() > 0 || thoughtSignature != "" {
			msg.ReasoningBlocks = append(msg.ReasoningBlocks, ReasoningBlock{
				Summary:   reasoningBuilder.String(),
				Signature: thoughtSignature,
			})
		}
		HydrateMessageReasoning(msg)
		ch <- StreamEvent{
			Type:    EventMessageDone,
			Message: msg,
		}
	})

	return ch, nil
}

func (p *GeminiProvider) Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	client, err := conn.GetGeminiClient(ctx, req.BaseURL, req.APIKey)
	if err != nil {
		return nil, err
	}

	if req.Model == "" {
		return nil, fmt.Errorf("model is required for embedding")
	}

	contents := lo.Map(req.Texts, func(text string, _ int) *genai.Content {
		return genai.NewContentFromText(text, genai.RoleUser)
	})

	result, err := client.Models.EmbedContent(ctx, req.Model, contents, nil)
	if err != nil {
		return nil, fmt.Errorf("gemini embedding failed: %w", err)
	}

	embeddings := lo.Map(result.Embeddings, func(emb *genai.ContentEmbedding, _ int) []float32 {
		return emb.Values
	})

	return &EmbeddingResponse{Embeddings: embeddings}, nil
}

func (p *GeminiProvider) VectorNormalized() bool {
	return false
}
