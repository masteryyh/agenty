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
	"encoding/base64"
	"fmt"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/models"
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
	config := &genai.GenerateContentConfig{}
	if len(req.Tools) > 0 {
		config.Tools = buildGeminiTools(req.Tools)
	}

	if req.ResponseFormat != nil && req.ResponseFormat.Type == "json_object" {
		config.ResponseMIMEType = "application/json"
	}

	if req.Thinking {
		thinkingConfig := &genai.ThinkingConfig{
			IncludeThoughts: true,
			ThinkingLevel:   validateThinkingLevel(req.ThinkingLevel),
		}
		config.ThinkingConfig = thinkingConfig
	}

	resp, err := client.Models.GenerateContent(ctx, req.Model, contents, config)
	if err != nil {
		return nil, err
	}

	result := &ChatResponse{}
	if resp.UsageMetadata != nil {
		result.TotalToken = int64(resp.UsageMetadata.TotalTokenCount)
	}

	thinkBlock := ReasoningBlock{}
	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		for _, part := range resp.Candidates[0].Content.Parts {
			if part.Text != "" {
				if part.Thought {
					thinkBlock.Summary = part.Text
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
				thinkBlock.Signature = base64.StdEncoding.EncodeToString(part.ThoughtSignature)
			}
		}
	}
	if thinkBlock.Summary != "" || thinkBlock.Signature != "" {
		result.ReasoningBlocks = append(result.ReasoningBlocks, thinkBlock)
	}

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
			return genai.NewContentFromText(msg.Content, genai.RoleUser), true

		default:
			return nil, false
		}
	})
}

func buildGeminiTools(defs []tools.ToolDefinition) []*genai.Tool {
	decls := lo.Map(defs, func(def tools.ToolDefinition, _ int) *genai.FunctionDeclaration {
		properties := make(map[string]*genai.Schema)
		for name, prop := range def.Parameters.Properties {
			properties[name] = &genai.Schema{
				Type:        genai.Type(prop.Type),
				Description: prop.Description,
			}
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

func validateThinkingLevel(level string) genai.ThinkingLevel {
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
