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
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/providers"
	"github.com/masteryyh/agenty/pkg/tools"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"github.com/samber/lo"
)

const maxToolCallIterations = 20

type ChatExecutor struct {
	registry         *tools.Registry
	providerRegistry map[models.APIType]providers.Provider
}

var (
	chatExecutor *ChatExecutor
	once         sync.Once
)

func NewChatExecutor(registry *tools.Registry) *ChatExecutor {
	providerMap := map[models.APIType]providers.Provider{
		models.APITypeOpenAI:       providers.NewOpenAIProvider(),
		models.APITypeOpenAILegacy: providers.NewOpenAILegacyProvider(),
		models.APITypeAnthropic:    providers.NewAnthropicProvider(),
		models.APITypeKimi:         providers.NewKimiProvider(),
		models.APITypeGemini:       providers.NewGeminiProvider(),
		models.APITypeBigModel:     providers.NewBigModelProvider(),
		models.APITypeQwen:         providers.NewQwenProvider(),
	}

	return &ChatExecutor{
		registry:         registry,
		providerRegistry: providerMap,
	}
}

func GetChatExecutor() *ChatExecutor {
	once.Do(func() {
		chatExecutor = NewChatExecutor(tools.GetRegistry())
	})
	return chatExecutor
}

type ChatParams struct {
	Messages                  []providers.Message
	Model                     string
	AgentID                   uuid.UUID
	SessionID                 uuid.UUID
	ModelID                   uuid.UUID
	Cwd                       string
	Thinking                  bool
	ThinkingLevel             string
	AnthropicAdaptiveThinking bool
	BaseURL                   string
	APIKey                    string
	APIType                   models.APIType
	ResponseFormat            *providers.ResponseFormat
}

type ChatResult struct {
	TotalToken int64
	Messages   []providers.Message
}

func (ce *ChatExecutor) executeToolCallsParallel(ctx context.Context, tcc tools.ToolCallContext, toolCalls []models.ToolCall, onResult func(models.ToolResult)) []models.ToolResult {
	results := make([]models.ToolResult, len(toolCalls))
	var wg sync.WaitGroup

	for i, tc := range toolCalls {
		wg.Add(1)
		idx, call := i, tc
		safe.GoOnce(fmt.Sprintf("tool-%s-%s", call.Name, call.ID), func() {
			defer wg.Done()

			var result models.ToolResult
			func() {
				defer func() {
					if r := recover(); r != nil {
						slog.ErrorContext(ctx, "panic in tool execution", "tool", call.Name, "id", call.ID, "error", r)
						result = models.ToolResult{
							CallID:  call.ID,
							Name:    call.Name,
							Content: fmt.Sprintf("tool panicked: %v", r),
							IsError: true,
						}
					}
				}()
				slog.InfoContext(ctx, "executing tool", "name", call.Name, "id", call.ID)
				result = ce.registry.Execute(ctx, tcc, call)
				slog.InfoContext(ctx, "tool execution done", "name", call.Name, "id", call.ID, "isError", result.IsError)
			}()

			results[idx] = result
			if onResult != nil {
				onResult(result)
			}
		})
	}

	wg.Wait()
	return results
}

func (ce *ChatExecutor) Chat(ctx context.Context, params *ChatParams) (*ChatResult, error) {
	p, ok := ce.providerRegistry[params.APIType]
	if !ok {
		p = ce.providerRegistry[models.APITypeOpenAI]
	}

	toolDefs := ce.registry.Definitions()
	messages := make([]providers.Message, len(params.Messages))
	copy(messages, params.Messages)

	var totalTokens int64
	for i := range maxToolCallIterations {
		req := &providers.ChatRequest{
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

		assistantMsg := providers.Message{
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
			Cwd:       params.Cwd,
		}

		results := ce.executeToolCallsParallel(ctx, tcc, resp.ToolCalls, nil)
		for i := range results {
			toolMsg := providers.Message{
				Role:       models.RoleTool,
				Content:    results[i].Content,
				ToolResult: &results[i],
			}
			messages = append(messages, toolMsg)
		}
	}

	finalMessages := lo.Map(messages[len(params.Messages):], func(msg providers.Message, _ int) providers.Message {
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

func (ce *ChatExecutor) StreamChat(ctx context.Context, params *ChatParams) (<-chan providers.StreamEvent, error) {
	p, ok := ce.providerRegistry[params.APIType]
	if !ok {
		p = ce.providerRegistry[models.APITypeOpenAI]
	}

	toolDefs := ce.registry.Definitions()
	messages := make([]providers.Message, len(params.Messages))
	copy(messages, params.Messages)

	out := make(chan providers.StreamEvent, 64)

	safe.GoOnce("chat-executor-stream", func() {
		defer close(out)

		var totalTokens int64

		for i := range maxToolCallIterations {
			req := &providers.ChatRequest{
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
				out <- providers.StreamEvent{Type: providers.EventError, Error: err.Error()}
				return
			}

			var assistantMsg *providers.Message
			for evt := range providerCh {
				switch evt.Type {
				case providers.EventMessageDone:
					assistantMsg = evt.Message
				case providers.EventUsage:
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
				out <- providers.StreamEvent{Type: providers.EventError, Error: "no message received from provider"}
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
				Cwd:       params.Cwd,
			}

			onResult := func(result models.ToolResult) {
				r := result
				select {
				case out <- providers.StreamEvent{
					Type:       providers.EventToolResult,
					ToolResult: &r,
				}:
				case <-ctx.Done():
				}
			}

			results := ce.executeToolCallsParallel(ctx, tcc, assistantMsg.ToolCalls, onResult)
			for i := range results {
				toolMsg := providers.Message{
					Role:       models.RoleTool,
					Content:    results[i].Content,
					ToolResult: &results[i],
				}
				messages = append(messages, toolMsg)
			}
		}

		out <- providers.StreamEvent{
			Type:  providers.EventUsage,
			Usage: &providers.StreamUsage{TotalTokens: totalTokens},
		}

		out <- providers.StreamEvent{Type: providers.EventDone}
	})

	return out, nil
}
