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

package chat

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/chat/provider"
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"github.com/samber/lo"
)

const maxToolCallIterations = 20

type ChatExecutor struct {
	registry  *tools.Registry
	providers map[models.APIType]provider.ChatProvider
}

var (
	chatExecutor *ChatExecutor
	once         sync.Once
)

func NewChatExecutor(registry *tools.Registry) *ChatExecutor {
	providers := map[models.APIType]provider.ChatProvider{
		models.APITypeOpenAI:       provider.NewOpenAIProvider(),
		models.APITypeOpenAILegacy: provider.NewOpenAILegacyProvider(),
		models.APITypeAnthropic:    provider.NewAnthropicProvider(),
		models.APITypeKimi:         provider.NewKimiProvider(),
		models.APITypeGemini:       provider.NewGeminiProvider(),
		models.APITypeBigModel:     provider.NewBigModelProvider(),
	}

	return &ChatExecutor{
		registry:  registry,
		providers: providers,
	}
}

func GetChatExecutor() *ChatExecutor {
	once.Do(func() {
		chatExecutor = NewChatExecutor(tools.GetRegistry())
	})
	return chatExecutor
}

type ChatParams struct {
	Messages                  []provider.Message
	Model                     string
	AgentID                   uuid.UUID
	SessionID                 uuid.UUID
	ModelID                   uuid.UUID
	Thinking                  bool
	ThinkingLevel             string
	AnthropicAdaptiveThinking bool
	BaseURL                   string
	APIKey                    string
	APIType                   models.APIType
	ResponseFormat            *provider.ResponseFormat
}

type ChatResult struct {
	TotalToken int64
	Messages   []provider.Message
}

func (ce *ChatExecutor) Chat(ctx context.Context, params *ChatParams) (*ChatResult, error) {
	p, ok := ce.providers[params.APIType]
	if !ok {
		p = ce.providers[models.APITypeOpenAI]
	}

	toolDefs := ce.registry.Definitions()
	messages := make([]provider.Message, len(params.Messages))
	copy(messages, params.Messages)

	var totalTokens int64
	for i := range maxToolCallIterations {
		req := &provider.ChatRequest{
			Model:                     params.Model,
			Thinking:                  params.Thinking,
			ThinkingLevel:             params.ThinkingLevel,
			AnthropicAdaptiveThinking: params.AnthropicAdaptiveThinking,
			BigModelClearThinking:     i == 0,
			Messages:                  messages,
			Tools:                     toolDefs,
			BaseURL:                   params.BaseURL,
			APIType:                   params.APIType,
			APIKey:                    params.APIKey,
			ResponseFormat:            params.ResponseFormat,
		}

		resp, err := p.Chat(ctx, req)
		if err != nil {
			return nil, err
		}
		totalTokens += resp.TotalToken

		assistantMsg := provider.Message{
			Role:             models.RoleAssistant,
			Content:          resp.Content,
			ToolCalls:        resp.ToolCalls,
			ReasoningContent: resp.ReasoningContent,
			ReasoningBlocks:  resp.ReasoningBlocks,
		}

		messages = append(messages, assistantMsg)

		if len(resp.ToolCalls) == 0 {
			break
		}

		slog.InfoContext(ctx, "executing tool calls", "count", len(resp.ToolCalls), "iteration", i+1)

		tcc := tools.ToolCallContext{
			AgentID:   params.AgentID,
			SessionID: params.SessionID,
			ModelID:   params.ModelID,
			ModelCode: params.Model,
		}

		for _, tc := range resp.ToolCalls {
			slog.InfoContext(ctx, "executing tool", "name", tc.Name, "id", tc.ID)
			result := ce.registry.Execute(ctx, tcc, tc)

			toolMsg := provider.Message{
				Role:       models.RoleTool,
				Content:    result.Content,
				ToolResult: &result,
			}
			messages = append(messages, toolMsg)
		}
	}

	finalMessages := lo.Map(messages[len(params.Messages):], func(msg provider.Message, _ int) provider.Message {
		if msg.Role == models.RoleAssistant {
			if msg.ReasoningContent == "" && len(msg.ReasoningBlocks) > 0 {
				var summaryBuilder strings.Builder
				for _, block := range msg.ReasoningBlocks {
					if !block.Redacted {
						summaryBuilder.WriteString(block.Summary)
						summaryBuilder.WriteString("\n")
					}
				}
				msg.ReasoningContent = summaryBuilder.String()
			}
		} else {
			msg.ReasoningContent = ""
		}

		return msg
	})

	return &ChatResult{
		TotalToken: totalTokens,
		Messages:   finalMessages,
	}, nil
}

func (ce *ChatExecutor) StreamChat(ctx context.Context, params *ChatParams) (<-chan provider.StreamEvent, error) {
	p, ok := ce.providers[params.APIType]
	if !ok {
		p = ce.providers[models.APITypeOpenAI]
	}

	toolDefs := ce.registry.Definitions()
	messages := make([]provider.Message, len(params.Messages))
	copy(messages, params.Messages)

	out := make(chan provider.StreamEvent, 64)

	safe.GoOnce("chat-executor-stream", func() {
		defer close(out)

		var totalTokens int64

		for i := range maxToolCallIterations {
			req := &provider.ChatRequest{
				Model:                     params.Model,
				Thinking:                  params.Thinking,
				ThinkingLevel:             params.ThinkingLevel,
				AnthropicAdaptiveThinking: params.AnthropicAdaptiveThinking,
				BigModelClearThinking:     i == 0,
				Messages:                  messages,
				Tools:                     toolDefs,
				BaseURL:                   params.BaseURL,
				APIType:                   params.APIType,
				APIKey:                    params.APIKey,
				ResponseFormat:            params.ResponseFormat,
			}

			providerCh, err := p.StreamChat(ctx, req)
			if err != nil {
				out <- provider.StreamEvent{Type: provider.EventError, Error: err.Error()}
				return
			}

			var assistantMsg *provider.Message
			for evt := range providerCh {
				switch evt.Type {
				case provider.EventMessageDone:
					assistantMsg = evt.Message
				case provider.EventUsage:
					if evt.Usage != nil {
						totalTokens += evt.Usage.TotalTokens
					}
				}

				select {
				case out <- evt:
				case <-ctx.Done():
					return
				}
			}

			if assistantMsg == nil {
				out <- provider.StreamEvent{Type: provider.EventError, Error: "no message received from provider"}
				return
			}

			if assistantMsg.ReasoningContent == "" && len(assistantMsg.ReasoningBlocks) > 0 {
				var sb strings.Builder
				for _, block := range assistantMsg.ReasoningBlocks {
					if !block.Redacted {
						sb.WriteString(block.Summary)
						sb.WriteString("\n")
					}
				}
				assistantMsg.ReasoningContent = sb.String()
			}

			messages = append(messages, *assistantMsg)

			if len(assistantMsg.ToolCalls) == 0 {
				break
			}

			slog.InfoContext(ctx, "stream: executing tool calls", "count", len(assistantMsg.ToolCalls), "iteration", i+1)

			tcc := tools.ToolCallContext{
				AgentID:   params.AgentID,
				SessionID: params.SessionID,
				ModelID:   params.ModelID,
				ModelCode: params.Model,
			}

			for _, tc := range assistantMsg.ToolCalls {
				slog.InfoContext(ctx, "stream: executing tool", "name", tc.Name, "id", tc.ID)
				result := ce.registry.Execute(ctx, tcc, tc)

				select {
				case out <- provider.StreamEvent{
					Type:       provider.EventToolResult,
					ToolResult: &result,
				}:
				case <-ctx.Done():
					return
				}

				toolMsg := provider.Message{
					Role:       models.RoleTool,
					Content:    result.Content,
					ToolResult: &result,
				}
				messages = append(messages, toolMsg)
			}
		}

		out <- provider.StreamEvent{
			Type:  provider.EventUsage,
			Usage: &provider.StreamUsage{TotalTokens: totalTokens},
		}

		out <- provider.StreamEvent{Type: provider.EventDone}
	})

	return out, nil
}
