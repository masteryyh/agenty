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

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/customerrors"
)

const deepSeekMaxTokensDefault = 393216

type DeepSeekProvider struct{}

func NewDeepSeekProvider() *DeepSeekProvider {
	return &DeepSeekProvider{}
}

func (p *DeepSeekProvider) Name() string {
	return "deepseek"
}

func deepSeekEffortFromLevel(level string) string {
	if level == "max" {
		return "max"
	}
	return "high"
}

func (p *DeepSeekProvider) buildMessageParams(req *ChatRequest) anthropic.MessageNewParams {
	systemPrompts, messages := buildAnthropicMessages(req.Messages)
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		System:    systemPrompts,
		Messages:  messages,
		MaxTokens: deepSeekMaxTokensDefault,
	}

	if len(req.Tools) > 0 {
		params.Tools = buildAnthropicTools(req.Tools)
	}

	if req.Thinking {
		params.Thinking = anthropic.ThinkingConfigParamOfEnabled(31999)
	}

	return params
}

func (p *DeepSeekProvider) deepSeekOptions(req *ChatRequest) []option.RequestOption {
	if !req.Thinking {
		return nil
	}
	return []option.RequestOption{
		option.WithJSONSet("output_config", map[string]any{
			"effort": deepSeekEffortFromLevel(req.ThinkingLevel),
		}),
	}
}

func (p *DeepSeekProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	client := conn.GetAnthropicClient(req.BaseURL, req.APIKey)
	params := p.buildMessageParams(req)
	return anthropicChat(ctx, client, params, p.deepSeekOptions(req)...)
}

func (p *DeepSeekProvider) StreamChat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	client := conn.GetAnthropicClient(req.BaseURL, req.APIKey)
	params := p.buildMessageParams(req)
	return anthropicStreamChat(ctx, "deepseek-stream", client, params, p.deepSeekOptions(req)...)
}

func (p *DeepSeekProvider) Embed(_ context.Context, _ *EmbeddingRequest) (*EmbeddingResponse, error) {
	return nil, customerrors.ErrEmbeddingNotSupported
}

func (p *DeepSeekProvider) VectorNormalized() bool {
	return false
}
