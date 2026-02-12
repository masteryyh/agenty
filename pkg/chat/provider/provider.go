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
	"github.com/masteryyh/agenty/pkg/models"
)

type Message struct {
	Role                 models.MessageRole `json:"role"`
	Content              string             `json:"content"`
	ToolCalls            []models.ToolCall  `json:"toolCalls,omitempty"`
	ToolResult           *models.ToolResult `json:"toolResult,omitempty"`
	KimiReasoningContent string             `json:"kimiReasoningContent,omitempty"`
}

type ResponseFormat struct {
	Type       string            `json:"type"`                 // "json_object", "json_schema" or "text"
	JSONSchema *JSONSchemaFormat `json:"jsonSchema,omitempty"` // Required when Type is "json_schema"
}

type JSONSchemaFormat struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema"`
	Strict      bool           `json:"strict,omitempty"`
}

type ChatRequest struct {
	Model          string
	Messages       []Message
	Tools          []tools.ToolDefinition
	BaseURL        string
	APIKey         string
	MaxTokens      int64
	ResponseFormat *ResponseFormat
}

type ChatResponse struct {
	Content              string
	KimiReasoningContent string
	ToolCalls            []models.ToolCall
	TotalToken           int64
}

type ChatProvider interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	Name() string
}
