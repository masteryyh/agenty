package llm

import (
	"context"
	"fmt"

	json "github.com/bytedance/sonic"
	"github.com/openai/openai-go/v3"
	openaishared "github.com/openai/openai-go/v3/shared"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type openAIChatCaller struct {
	client *openai.Client
	model  catalog.Model
}

func (caller *openAIChatCaller) Invoke(ctx context.Context, request modelRequest) (*modelResponse, error) {
	params, err := caller.params(request)
	if err != nil {
		return nil, err
	}

	result, err := caller.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("llm: invoke OpenAI Chat Completions API: %w", err)
	}

	return openAIChatResponse(result)
}

func (caller *openAIChatCaller) Stream(
	ctx context.Context,
	request modelRequest,
	handler modelStreamHandler,
) (*modelResponse, error) {
	params, err := caller.params(request)
	if err != nil {
		return nil, err
	}

	stream := caller.client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()

	var accumulator openai.ChatCompletionAccumulator
	for stream.Next() {
		chunk := stream.Current()
		if !accumulator.AddChunk(chunk) {
			return nil, fmt.Errorf("llm: accumulate OpenAI Chat Completions stream chunk")
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				if err := emit(handler, modelStreamEvent{
					Type: modelStreamEventTextDelta, Index: int(choice.Index), Delta: choice.Delta.Content,
				}); err != nil {
					return nil, err
				}
			}
			for _, toolCall := range choice.Delta.ToolCalls {
				if toolCall.ID != "" || toolCall.Function.Name != "" {
					if err := emit(handler, modelStreamEvent{
						Type: modelStreamEventToolUseStart, Index: int(toolCall.Index),
						ToolUseID: toolCall.ID, ToolName: toolCall.Function.Name,
					}); err != nil {
						return nil, err
					}
				}
				if toolCall.Function.Arguments != "" {
					if err := emit(handler, modelStreamEvent{
						Type: modelStreamEventToolInputDelta, Index: int(toolCall.Index),
						Delta: toolCall.Function.Arguments,
					}); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("llm: stream OpenAI Chat Completions API: %w", err)
	}

	final, err := openAIChatResponse(&accumulator.ChatCompletion)
	if err != nil {
		return nil, err
	}
	for index, block := range final.Content {
		tool, ok := block.(conversation.ToolUseBlock)
		if !ok {
			continue
		}
		if err := emit(handler, modelStreamEvent{
			Type: modelStreamEventToolUseDone, Index: index,
			ToolUseID: tool.ID, ToolName: tool.Name, ToolInput: tool.Input,
		}); err != nil {
			return nil, err
		}
	}
	if err := emit(handler, modelStreamEvent{Type: modelStreamEventCompleted, Response: final}); err != nil {
		return nil, err
	}

	return final, nil
}

func (caller *openAIChatCaller) params(request modelRequest) (openai.ChatCompletionNewParams, error) {
	if err := validateRequest(request); err != nil {
		return openai.ChatCompletionNewParams{}, err
	}

	prompt, err := systemPrompt(request)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}
	effort, err := nativeReasoningEffort(caller.model, request.ReasoningEffort)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}

	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(request.Messages)+1)
	if prompt != "" {
		messages = append(messages, openai.SystemMessage(prompt))
	}
	for index, message := range request.Messages {
		if message.Role == conversation.RoleSystem {
			continue
		}
		converted, err := openAIChatMessages(message)
		if err != nil {
			return openai.ChatCompletionNewParams{}, fmt.Errorf("llm: convert OpenAI Chat message %d: %w", index, err)
		}
		messages = append(messages, converted...)
	}

	tools := make([]openai.ChatCompletionToolUnionParam, 0, len(request.Tools))
	for _, tool := range request.Tools {
		converted, err := openAIChatToolDefinition(tool)
		if err != nil {
			return openai.ChatCompletionNewParams{}, err
		}
		tools = append(tools, converted)
	}

	params := openai.ChatCompletionNewParams{
		Model:               openaishared.ChatModel(caller.model.Slug.String()),
		Messages:            messages,
		MaxCompletionTokens: openai.Int(request.MaxOutputTokens),
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		},
		Tools: tools,
	}
	if effort != "" {
		params.ReasoningEffort = openaishared.ReasoningEffort(effort)
	}

	return params, nil
}

func openAIChatToolDefinition(tool modelToolDefinition) (openai.ChatCompletionToolUnionParam, error) {
	schema, err := toolSchemaMap(tool.InputSchema)
	if err != nil {
		return openai.ChatCompletionToolUnionParam{}, fmt.Errorf("llm: convert OpenAI Chat tool %q schema: %w", tool.Name, err)
	}

	converted := openaishared.FunctionDefinitionParam{
		Name:        tool.Name,
		Description: openai.String(tool.Description),
		Parameters:  openaishared.FunctionParameters(schema),
		Strict:      openai.Bool(tool.Strict),
	}

	return openai.ChatCompletionFunctionTool(converted), nil
}

func openAIChatMessages(message conversation.Message) ([]openai.ChatCompletionMessageParamUnion, error) {
	if message.Role == conversation.RoleUser {
		parts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(message.Content))
		messages := make([]openai.ChatCompletionMessageParamUnion, 0, 2)
		for _, block := range message.Content {
			switch value := block.(type) {
			case conversation.TextBlock:
				parts = append(parts, openai.TextContentPart(value.Text))
			case conversation.ImageBlock:
				url, err := imageURL(value)
				if err != nil {
					return nil, err
				}
				parts = append(parts, openai.ImageContentPart(
					openai.ChatCompletionContentPartImageImageURLParam{URL: url, Detail: "auto"},
				))
			case conversation.ToolResultBlock:
				if len(parts) > 0 {
					messages = append(messages, openai.UserMessage(parts))
					parts = nil
				}
				output, err := textContent(value.Content)
				if err != nil {
					return nil, err
				}
				messages = append(messages, openai.ToolMessage(output, value.ToolUseID))
			default:
				return nil, unsupportedContent("OpenAI Chat user message cannot contain %q", block.BlockType())
			}
		}
		if len(parts) > 0 {
			messages = append(messages, openai.UserMessage(parts))
		}
		return messages, nil
	}

	var text string
	toolCalls := make([]openai.ChatCompletionMessageToolCallUnionParam, 0)
	for _, block := range message.Content {
		switch value := block.(type) {
		case conversation.TextBlock:
			text += value.Text
		case conversation.ToolUseBlock:
			if _, err := rawObject(value.Input, "tool input"); err != nil {
				return nil, err
			}
			call := openai.ChatCompletionMessageFunctionToolCallParam{
				ID: value.ID,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name: value.Name, Arguments: string(value.Input),
				},
			}
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{OfFunction: &call})
		default:
			return nil, unsupportedContent("OpenAI Chat assistant message cannot contain %q", block.BlockType())
		}
	}

	assistant := openai.AssistantMessage(text)
	assistant.OfAssistant.ToolCalls = toolCalls
	return []openai.ChatCompletionMessageParamUnion{assistant}, nil
}

func openAIChatResponse(result *openai.ChatCompletion) (*modelResponse, error) {
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("llm: OpenAI Chat Completions returned no choices")
	}

	choice := result.Choices[0]
	content := make(conversation.Content, 0, len(choice.Message.ToolCalls)+1)
	if choice.Message.Content != "" {
		content = append(content, conversation.TextBlock{Text: choice.Message.Content})
	} else if choice.Message.Refusal != "" {
		content = append(content, conversation.TextBlock{Text: choice.Message.Refusal})
	}
	for _, call := range choice.Message.ToolCalls {
		if call.Type != "function" {
			continue
		}
		arguments := shared.RawJSON(call.Function.Arguments)
		if !json.Valid(arguments) {
			return nil, fmt.Errorf("llm: OpenAI Chat returned invalid tool arguments for %q", call.Function.Name)
		}
		content = append(content, conversation.ToolUseBlock{
			ID: call.ID, Name: call.Function.Name, Input: arguments,
		})
	}

	return &modelResponse{
		ID: result.ID, Model: result.Model, Content: content,
		StopReason: openAIChatStopReason(choice.FinishReason),
		Usage: conversation.TokenUsage{
			Input: result.Usage.PromptTokens, Output: result.Usage.CompletionTokens,
			CachedRead: result.Usage.PromptTokensDetails.CachedTokens,
			CacheWrite: result.Usage.PromptTokensDetails.CacheWriteTokens,
			Reasoning:  result.Usage.CompletionTokensDetails.ReasoningTokens,
			Total:      result.Usage.TotalTokens,
		},
	}, nil
}

func openAIChatStopReason(reason string) modelStopReason {
	switch reason {
	case "length":
		return modelStopReasonMaxTokens
	case "tool_calls", "function_call":
		return modelStopReasonToolUse
	case "content_filter":
		return modelStopReasonContentFilter
	default:
		return modelStopReasonEndTurn
	}
}
