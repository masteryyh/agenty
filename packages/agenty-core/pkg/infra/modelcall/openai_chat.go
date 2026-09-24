package modelcall

import (
	"context"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	openaishared "github.com/openai/openai-go/v3/shared"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type openAIChatCaller struct {
	client *openai.Client
	model  ModelCallConfig
}

func (caller *openAIChatCaller) Invoke(ctx context.Context, request ModelCallRequest) (*ModelCallResponse, error) {
	params, err := caller.params(request)
	if err != nil {
		return nil, err
	}

	result, err := caller.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("modelcall: invoke OpenAI Chat Completions API: %w", err)
	}

	return openAIChatResponse(result)
}

func (caller *openAIChatCaller) Stream(
	ctx context.Context,
	request ModelCallRequest,
	handler ModelCallStreamHandler,
) (*ModelCallResponse, error) {
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
			return nil, fmt.Errorf("modelcall: accumulate OpenAI Chat Completions stream chunk")
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				if err := emit(handler, ModelCallStreamEvent{
					Type: ModelCallStreamEventTextDelta, Index: int(choice.Index), Delta: choice.Delta.Content,
				}); err != nil {
					return nil, err
				}
			}
			for _, toolCall := range choice.Delta.ToolCalls {
				if toolCall.ID != "" || toolCall.Function.Name != "" {
					if err := emit(handler, ModelCallStreamEvent{
						Type: ModelCallStreamEventToolUseStart, Index: int(toolCall.Index),
						ToolUseID: toolCall.ID, ToolName: toolCall.Function.Name,
					}); err != nil {
						return nil, err
					}
				}
				if toolCall.Function.Arguments != "" {
					if err := emit(handler, ModelCallStreamEvent{
						Type: ModelCallStreamEventToolInputDelta, Index: int(toolCall.Index),
						Delta: toolCall.Function.Arguments,
					}); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("modelcall: stream OpenAI Chat Completions API: %w", err)
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
		if err := emit(handler, ModelCallStreamEvent{
			Type: ModelCallStreamEventToolUseDone, Index: index,
			ToolUseID: tool.ID, ToolName: tool.Name, ToolInput: tool.Input,
		}); err != nil {
			return nil, err
		}
	}
	if err := emit(handler, ModelCallStreamEvent{Type: ModelCallStreamEventCompleted, Response: final}); err != nil {
		return nil, err
	}

	return final, nil
}

func (caller *openAIChatCaller) params(request ModelCallRequest) (openai.ChatCompletionNewParams, error) {
	if err := validateRequest(request); err != nil {
		return openai.ChatCompletionNewParams{}, err
	}

	prompt, err := systemPrompt(request)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}
	effort, err := modelReasoningEffort(caller.model, request.ReasoningEffort)
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
			return openai.ChatCompletionNewParams{}, fmt.Errorf("modelcall: convert OpenAI Chat message %d: %w", index, err)
		}
		messages = append(messages, converted...)
	}

	tools, err := openAIChatTools(request.Tools)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}

	params := openai.ChatCompletionNewParams{
		Model:               openaishared.ChatModel(caller.model.ModelCode),
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

	if request.OutputFormat != nil {
		schema, err := request.OutputFormat.Schema.toMap()
		if err != nil {
			return openai.ChatCompletionNewParams{}, err
		}
		params.ResponseFormat.OfJSONSchema = &openaishared.ResponseFormatJSONSchemaParam{
			JSONSchema: openaishared.ResponseFormatJSONSchemaJSONSchemaParam{
				Name: request.OutputFormat.Name, Schema: schema, Strict: openai.Bool(true),
			},
		}
	}

	return params, nil
}

func openAIChatTools(definitions []ToolDefinition) ([]openai.ChatCompletionToolUnionParam, error) {
	tools := make([]openai.ChatCompletionToolUnionParam, 0, len(definitions))
	for _, definition := range definitions {
		if definition.Type == ToolTypeApplyPatch {
			continue
		}
		tool, err := openAIChatToolDefinition(definition)
		if err != nil {
			return nil, err
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func openAIChatToolDefinition(tool ToolDefinition) (openai.ChatCompletionToolUnionParam, error) {
	if _, err := providerToolType(tool); err != nil {
		return openai.ChatCompletionToolUnionParam{}, err
	}
	schema, err := toolSchemaMap(tool.InputSchema)
	if err != nil {
		return openai.ChatCompletionToolUnionParam{}, fmt.Errorf("modelcall: convert OpenAI Chat tool %q schema: %w", tool.Name, err)
	}

	converted := openaishared.FunctionDefinitionParam{
		Name:        tool.Name,
		Description: openai.String(tool.Description),
		Parameters:  openaishared.FunctionParameters(schema),
		Strict:      openai.Bool(tool.Strict),
	}

	return openai.ChatCompletionFunctionTool(converted), nil
}

func openAIChatMessages(message ModelCallMessage) ([]openai.ChatCompletionMessageParamUnion, error) {
	if message.Role == conversation.RoleDeveloper {
		text, err := textContent(message.Content)
		if err != nil {
			return nil, err
		}
		return []openai.ChatCompletionMessageParamUnion{openai.DeveloperMessage(text)}, nil
	}
	if message.Role == conversation.RoleUser {
		parts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(message.Content))
		toolImages := make([]openai.ChatCompletionContentPartUnionParam, 0)
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
				output, images, err := openAIChatToolResult(value.Content)
				if err != nil {
					return nil, err
				}
				messages = append(messages, openai.ToolMessage(output, value.ToolUseID))
				toolImages = append(toolImages, images...)
			default:
				return nil, unsupportedContent("OpenAI Chat user message cannot contain %q", block.BlockType())
			}
		}
		if len(parts) > 0 {
			messages = append(messages, openai.UserMessage(parts))
		}
		if len(toolImages) > 0 {
			messages = append(messages, openai.UserMessage(toolImages))
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
		case conversation.ShellCallBlock:
			call := value.ToolUseBlock()
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: call.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name: call.Name, Arguments: string(call.Input),
					},
				},
			})
		case conversation.ApplyPatchCallBlock:
			call := value.ToolUseBlock()
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: call.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name: call.Name, Arguments: string(call.Input),
					},
				},
			})
		default:
			return nil, unsupportedContent("OpenAI Chat assistant message cannot contain %q", block.BlockType())
		}
	}

	assistant := openai.AssistantMessage(text)
	assistant.OfAssistant.ToolCalls = toolCalls
	return []openai.ChatCompletionMessageParamUnion{assistant}, nil
}

func openAIChatToolResult(content conversation.Content) (string, []openai.ChatCompletionContentPartUnionParam, error) {
	var text strings.Builder
	images := make([]openai.ChatCompletionContentPartUnionParam, 0)
	for _, block := range content {
		switch value := block.(type) {
		case conversation.TextBlock:
			text.WriteString(value.Text)
		case conversation.ShellCallOutputBlock:
			encoded, err := marshalShellCallOutput(value)
			if err != nil {
				return "", nil, fmt.Errorf("encode shell output: %w", err)
			}
			text.WriteString(encoded)
		case conversation.ImageBlock:
			url, err := imageURL(value)
			if err != nil {
				return "", nil, err
			}
			images = append(images, openai.ImageContentPart(
				openai.ChatCompletionContentPartImageImageURLParam{URL: url, Detail: "auto"},
			))
		default:
			return "", nil, unsupportedContent("OpenAI Chat tool result cannot contain %q", block.BlockType())
		}
	}
	output := text.String()
	if output == "" && len(images) > 0 {
		output = "Tool returned image content."
	}
	return output, images, nil
}

func openAIChatResponse(result *openai.ChatCompletion) (*ModelCallResponse, error) {
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("modelcall: OpenAI Chat Completions returned no choices")
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
		content = append(content, structuredToolUseBlock(
			call.ID,
			call.Function.Name,
			shared.RawJSON(call.Function.Arguments),
		))
	}

	return &ModelCallResponse{
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

func openAIChatStopReason(reason string) ModelCallStopReason {
	switch reason {
	case "length":
		return ModelCallStopReasonMaxTokens
	case "tool_calls", "function_call":
		return ModelCallStopReasonToolUse
	case "content_filter":
		return ModelCallStopReasonContentFilter
	default:
		return ModelCallStopReasonEndTurn
	}
}
