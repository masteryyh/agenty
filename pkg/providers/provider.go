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
	"strings"

	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/tools"
)

type StreamEventType string

const (
	EventReasoningDelta StreamEventType = "reasoning_delta"
	EventContentDelta   StreamEventType = "content_delta"
	EventToolCallStart  StreamEventType = "tool_call_start"
	EventToolCallDelta  StreamEventType = "tool_call_delta"
	EventToolCallDone   StreamEventType = "tool_call_done"
	EventToolResult     StreamEventType = "tool_result"
	EventMessageDone    StreamEventType = "message_done"
	EventUsage          StreamEventType = "usage"
	EventError          StreamEventType = "error"
	EventDone           StreamEventType = "done"
	EventModelSwitch    StreamEventType = "model_switch"
)

type StreamEvent struct {
	Type                StreamEventType    `json:"type"`
	Content             string             `json:"content,omitempty"`
	Reasoning           string             `json:"reasoning,omitempty"`
	ToolCall            *models.ToolCall   `json:"toolCall,omitempty"`
	ToolResult          *models.ToolResult `json:"toolResult,omitempty"`
	Usage               *StreamUsage       `json:"usage,omitempty"`
	Error               string             `json:"error,omitempty"`
	Message             *Message           `json:"message,omitempty"`
	ModelID             string             `json:"modelId,omitempty"`
	ModelName           string             `json:"modelName,omitempty"`
	ModelThinking       bool               `json:"modelThinking,omitempty"`
	ModelThinkingLevels []string           `json:"modelThinkingLevels,omitempty"`
}

type StreamUsage struct {
	TotalTokens int64 `json:"totalTokens"`
}

type ReasoningBlock struct {
	Summary   string `json:"summary"`
	Signature string `json:"signature,omitempty"`
	Redacted  bool   `json:"redacted,omitempty"`
}

type Message struct {
	Role                    models.MessageRole `json:"role"`
	Content                 string             `json:"content"`
	ToolCalls               []models.ToolCall  `json:"toolCalls,omitempty"`
	ToolResult              *models.ToolResult `json:"toolResult,omitempty"`
	ReasoningContent        string             `json:"reasoningContent,omitempty"`
	ReasoningDurationMillis int64              `json:"reasoningDurationMillis,omitempty"`
	ReasoningBlocks         []ReasoningBlock   `json:"reasoningBlocks,omitempty"`
}

type ResponseFormat struct {
	Type       string            `json:"type"`
	JSONSchema *JSONSchemaFormat `json:"jsonSchema,omitempty"`
}

type JSONSchemaFormat struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema"`
	Strict      bool           `json:"strict,omitempty"`
}

type ChatRequest struct {
	Model                     string
	Messages                  []Message
	Tools                     []tools.ToolDefinition
	Thinking                  bool
	ThinkingLevel             string
	AnthropicAdaptiveThinking bool
	BigModelClearThinking     bool
	BaseURL                   string
	APIType                   models.APIType
	APIKey                    string
	MaxTokens                 int64
	ResponseFormat            *ResponseFormat
}

type ChatResponse struct {
	Content          string
	ReasoningContent string
	ReasoningBlocks  []ReasoningBlock
	ToolCalls        []models.ToolCall
	TotalToken       int64
}

func reasoningContentFromBlocks(blocks []ReasoningBlock) string {
	summaries := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Redacted {
			continue
		}
		summary := strings.TrimSpace(block.Summary)
		if summary == "" {
			continue
		}
		summaries = append(summaries, summary)
	}
	return strings.Join(summaries, "\n")
}

func HydrateMessageReasoning(msg *Message) {
	if msg == nil || msg.ReasoningContent != "" {
		return
	}
	msg.ReasoningContent = reasoningContentFromBlocks(msg.ReasoningBlocks)
}

func hydrateChatResponseReasoning(resp *ChatResponse) {
	if resp == nil || resp.ReasoningContent != "" {
		return
	}
	resp.ReasoningContent = reasoningContentFromBlocks(resp.ReasoningBlocks)
}

type EmbeddingRequest struct {
	Model      string
	Texts      []string
	BaseURL    string
	APIKey     string
	Dimensions int64
}

type EmbeddingResponse struct {
	Embeddings [][]float32
}

type Provider interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	StreamChat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error)
	Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error)
	Name() string
	VectorNormalized() bool
}

var ModelProviders = map[models.APIType]Provider{
	models.APITypeOpenAI:       NewOpenAIProvider(),
	models.APITypeOpenAILegacy: NewOpenAILegacyProvider(),
	models.APITypeAnthropic:    NewAnthropicProvider(),
	models.APITypeKimi:         NewKimiProvider(),
	models.APITypeGemini:       NewGeminiProvider(),
	models.APITypeBigModel:     NewBigModelProvider(),
	models.APITypeQwen:         NewQwenProvider(),
	models.APITypeDeepSeek:     NewDeepSeekProvider(),
}
