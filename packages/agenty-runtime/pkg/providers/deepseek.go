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
