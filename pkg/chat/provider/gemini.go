package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/masteryyh/agenty/pkg/chat/tools"
)

type GeminiProvider struct {
	httpClient *http.Client
}

func NewGeminiProvider() *GeminiProvider {
	return &GeminiProvider{
		httpClient: &http.Client{},
	}
}

func (p *GeminiProvider) Name() string {
	return "gemini"
}

type geminiRequest struct {
	Contents []geminiContent       `json:"contents"`
	Tools    []geminiToolContainer `json:"tools,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string               `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall  `json:"functionCall,omitempty"`
	FunctionResponse *geminiFuncResponse  `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type geminiFuncResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiToolContainer struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsage      `json:"usageMetadata"`
	Error         *geminiError      `json:"error,omitempty"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

type geminiUsage struct {
	PromptTokenCount     int64 `json:"promptTokenCount"`
	CandidatesTokenCount int64 `json:"candidatesTokenCount"`
	TotalTokenCount      int64 `json:"totalTokenCount"`
}

type geminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (p *GeminiProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}

	contents := buildGeminiContents(req.Messages)
	apiReq := geminiRequest{
		Contents: contents,
	}

	if len(req.Tools) > 0 {
		apiReq.Tools = buildGeminiTools(req.Tools)
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", baseURL, req.Model, req.APIKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", httpResp.StatusCode, string(respBody))
	}

	var apiResp geminiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("API error: %s", apiResp.Error.Message)
	}

	result := &ChatResponse{}
	if apiResp.UsageMetadata != nil {
		result.TotalToken = apiResp.UsageMetadata.TotalTokenCount
	}

	if len(apiResp.Candidates) > 0 {
		candidate := apiResp.Candidates[0]
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				if result.Content != "" {
					result.Content += "\n"
				}
				result.Content += part.Text
			}
			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				result.ToolCalls = append(result.ToolCalls, tools.ToolCall{
					ID:        fmt.Sprintf("call_%s", part.FunctionCall.Name),
					Name:      part.FunctionCall.Name,
					Arguments: argsJSON,
				})
			}
		}
	}

	return result, nil
}

func buildGeminiContents(messages []Message) []geminiContent {
	var result []geminiContent
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			result = append(result, geminiContent{
				Role:  "user",
				Parts: []geminiPart{{Text: msg.Content}},
			})
		case "assistant":
			gc := geminiContent{
				Role: "model",
			}
			if msg.Content != "" {
				gc.Parts = append(gc.Parts, geminiPart{Text: msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				var args map[string]any
				json.Unmarshal(tc.Arguments, &args)
				gc.Parts = append(gc.Parts, geminiPart{
					FunctionCall: &geminiFunctionCall{
						Name: tc.Name,
						Args: args,
					},
				})
			}
			result = append(result, gc)
		case "tool":
			if msg.ToolResult != nil {
				result = append(result, geminiContent{
					Role: "user",
					Parts: []geminiPart{
						{
							FunctionResponse: &geminiFuncResponse{
								Name: msg.ToolResult.Name,
								Response: map[string]any{
									"result": msg.ToolResult.Content,
								},
							},
						},
					},
				})
			}
		case "system":
			result = append(result, geminiContent{
				Role:  "user",
				Parts: []geminiPart{{Text: msg.Content}},
			})
		}
	}
	return result
}

func buildGeminiTools(defs []tools.ToolDefinition) []geminiToolContainer {
	decls := make([]geminiFunctionDecl, len(defs))
	for i, def := range defs {
		properties := make(map[string]any)
		for name, prop := range def.Parameters.Properties {
			properties[name] = map[string]any{
				"type":        prop.Type,
				"description": prop.Description,
			}
		}
		decls[i] = geminiFunctionDecl{
			Name:        def.Name,
			Description: def.Description,
			Parameters: map[string]any{
				"type":       def.Parameters.Type,
				"properties": properties,
				"required":   def.Parameters.Required,
			},
		}
	}
	return []geminiToolContainer{{FunctionDeclarations: decls}}
}
