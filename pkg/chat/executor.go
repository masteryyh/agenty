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
	"sync"

	"github.com/masteryyh/agenty/pkg/chat/provider"
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/models"
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
		models.APITypeOpenAI:    provider.NewOpenAIProvider(),
		models.APITypeAnthropic: provider.NewAnthropicProvider(),
		models.APITypeKimi:      provider.NewKimiProvider(),
		models.APITypeGemini:    provider.NewGeminiProvider(),
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
	Messages       []provider.Message
	Model          string
	BaseURL        string
	APIKey         string
	APIType        models.APIType
	ResponseFormat *provider.ResponseFormat
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
			Model:          params.Model,
			Messages:       messages,
			Tools:          toolDefs,
			BaseURL:        params.BaseURL,
			APIKey:         params.APIKey,
			ResponseFormat: params.ResponseFormat,
		}

		resp, err := p.Chat(ctx, req)
		if err != nil {
			return nil, err
		}
		totalTokens += resp.TotalToken

		assistantMsg := provider.Message{
			Role:      models.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		if params.APIType == models.APITypeKimi {
			assistantMsg.KimiReasoningContent = resp.KimiReasoningContent
		}

		messages = append(messages, assistantMsg)

		if len(resp.ToolCalls) == 0 {
			return &ChatResult{
				TotalToken: totalTokens,
				Messages:   messages[len(params.Messages):],
			}, nil
		}

		slog.InfoContext(ctx, "executing tool calls", "count", len(resp.ToolCalls), "iteration", i+1)

		for _, tc := range resp.ToolCalls {
			slog.InfoContext(ctx, "executing tool", "name", tc.Name, "id", tc.ID)
			result := ce.registry.Execute(ctx, tc)

			toolMsg := provider.Message{
				Role:       models.RoleTool,
				Content:    result.Content,
				ToolResult: &result,
			}
			if params.APIType == models.APITypeKimi {
				toolMsg.KimiReasoningContent = resp.KimiReasoningContent
			}
			messages = append(messages, toolMsg)
		}
	}

	return &ChatResult{
		TotalToken: totalTokens,
		Messages:   messages[len(params.Messages):],
	}, nil
}
