package provider

import (
	"context"
	"fmt"

	"github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/conn"
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
	var config *genai.GenerateContentConfig
	if len(req.Tools) > 0 {
		config = &genai.GenerateContentConfig{
			Tools: buildGeminiTools(req.Tools),
		}
	}

	resp, err := client.Models.GenerateContent(ctx, req.Model, contents, config)
	if err != nil {
		return nil, err
	}

	result := &ChatResponse{}
	if resp.UsageMetadata != nil {
		result.TotalToken = int64(resp.UsageMetadata.TotalTokenCount)
	}

	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		for _, part := range resp.Candidates[0].Content.Parts {
			if part.Text != "" {
				if result.Content != "" {
					result.Content += "\n"
				}
				result.Content += part.Text
			}
			if part.FunctionCall != nil {
				argsJSON, err := sonic.Marshal(part.FunctionCall.Args)
				if err != nil {
					argsJSON = []byte("{}")
				}
				id := part.FunctionCall.ID
				if id == "" {
					id = fmt.Sprintf("call_%s", part.FunctionCall.Name)
				}
				result.ToolCalls = append(result.ToolCalls, tools.ToolCall{
					ID:        id,
					Name:      part.FunctionCall.Name,
					Arguments: argsJSON,
				})
			}
		}
	}

	return result, nil
}

func buildGeminiContents(messages []Message) []*genai.Content {
	return lo.FilterMap(messages, func(msg Message, _ int) (*genai.Content, bool) {
		switch msg.Role {
		case "user":
			return genai.NewContentFromText(msg.Content, genai.RoleUser), true
		case "assistant":
			c := &genai.Content{Role: genai.RoleModel}
			if msg.Content != "" {
				c.Parts = append(c.Parts, genai.NewPartFromText(msg.Content))
			}
			for _, tc := range msg.ToolCalls {
				var args map[string]any
				if err := sonic.Unmarshal(tc.Arguments, &args); err != nil {
					args = map[string]any{}
				}
				c.Parts = append(c.Parts, genai.NewPartFromFunctionCall(tc.Name, args))
			}
			return c, true
		case "tool":
			if msg.ToolResult != nil {
				return genai.NewContentFromFunctionResponse(
					msg.ToolResult.Name,
					map[string]any{"result": msg.ToolResult.Content},
					genai.RoleUser,
				), true
			}
			return nil, false
		case "system":
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
