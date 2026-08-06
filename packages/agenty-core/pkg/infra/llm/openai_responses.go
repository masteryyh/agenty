package llm

import (
	"context"
	"fmt"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	openaishared "github.com/openai/openai-go/v3/shared"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type openAIResponsesCaller struct {
	client *openai.Client
	model  catalog.Model
}

func (caller *openAIResponsesCaller) Invoke(ctx context.Context, request modelRequest) (*modelResponse, error) {
	params, err := caller.params(request)
	if err != nil {
		return nil, err
	}

	result, err := caller.client.Responses.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("llm: invoke OpenAI Responses API: %w", err)
	}

	return openAIResponsesResponse(result)
}

func (caller *openAIResponsesCaller) Stream(
	ctx context.Context,
	request modelRequest,
	handler modelStreamHandler,
) (*modelResponse, error) {
	params, err := caller.params(request)
	if err != nil {
		return nil, err
	}

	stream := caller.client.Responses.NewStreaming(ctx, params)
	defer stream.Close()

	var final *modelResponse
	for stream.Next() {
		switch event := stream.Current().AsAny().(type) {
		case responses.ResponseTextDeltaEvent:
			err = emit(handler, modelStreamEvent{
				Type: modelStreamEventTextDelta, Index: int(event.OutputIndex), Delta: event.Delta,
			})
		case responses.ResponseReasoningSummaryTextDeltaEvent:
			err = emit(handler, modelStreamEvent{
				Type: modelStreamEventReasoningDelta, Index: int(event.OutputIndex), Delta: event.Delta,
			})
		case responses.ResponseReasoningTextDeltaEvent:
			err = emit(handler, modelStreamEvent{
				Type: modelStreamEventReasoningDelta, Index: int(event.OutputIndex), Delta: event.Delta,
			})
		case responses.ResponseFunctionCallArgumentsDeltaEvent:
			err = emit(handler, modelStreamEvent{
				Type: modelStreamEventToolInputDelta, Index: int(event.OutputIndex), Delta: event.Delta,
			})
		case responses.ResponseOutputItemAddedEvent:
			if item, ok := event.Item.AsAny().(responses.ResponseFunctionToolCall); ok {
				err = emit(handler, modelStreamEvent{
					Type: modelStreamEventToolUseStart, Index: int(event.OutputIndex),
					ToolUseID: item.CallID, ToolName: item.Name,
				})
			}
		case responses.ResponseOutputItemDoneEvent:
			if item, ok := event.Item.AsAny().(responses.ResponseFunctionToolCall); ok {
				err = emit(handler, modelStreamEvent{
					Type: modelStreamEventToolUseDone, Index: int(event.OutputIndex),
					ToolUseID: item.CallID, ToolName: item.Name,
					ToolInput: shared.RawJSON(item.Arguments),
				})
			}
		case responses.ResponseCompletedEvent:
			final, err = openAIResponsesResponse(&event.Response)
		}
		if err != nil {
			return nil, err
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("llm: stream OpenAI Responses API: %w", err)
	}
	if final == nil {
		return nil, fmt.Errorf("llm: OpenAI Responses stream ended without a completed response")
	}
	if err := emit(handler, modelStreamEvent{Type: modelStreamEventCompleted, Response: final}); err != nil {
		return nil, err
	}

	return final, nil
}

func (caller *openAIResponsesCaller) params(request modelRequest) (responses.ResponseNewParams, error) {
	if err := validateRequest(request); err != nil {
		return responses.ResponseNewParams{}, err
	}

	instructions, err := systemPrompt(request)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}
	effort, err := nativeReasoningEffort(caller.model, request.ReasoningEffort)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}

	input := make(responses.ResponseInputParam, 0, len(request.Messages))
	for index, message := range request.Messages {
		if message.Role == conversation.RoleSystem {
			continue
		}
		items, err := openAIResponsesMessage(message)
		if err != nil {
			return responses.ResponseNewParams{}, fmt.Errorf("llm: convert OpenAI Responses message %d: %w", index, err)
		}
		input = append(input, items...)
	}

	tools := make([]responses.ToolUnionParam, 0, len(request.Tools))
	for _, tool := range request.Tools {
		converted, err := openAIResponsesToolDefinition(tool)
		if err != nil {
			return responses.ResponseNewParams{}, err
		}
		tools = append(tools, converted)
	}

	params := responses.ResponseNewParams{
		Model:           openaishared.ResponsesModel(caller.model.Slug.String()),
		Input:           responses.ResponseNewParamsInputUnion{OfInputItemList: input},
		MaxOutputTokens: openai.Int(request.MaxOutputTokens),
		Store:           openai.Bool(false),
		Tools:           tools,
	}
	if instructions != "" {
		params.Instructions = openai.String(instructions)
	}
	if effort != "" {
		params.Reasoning.Effort = openaishared.ReasoningEffort(effort)
		params.Include = []responses.ResponseIncludable{
			responses.ResponseIncludableReasoningEncryptedContent,
		}
	}

	return params, nil
}

func openAIResponsesToolDefinition(tool modelToolDefinition) (responses.ToolUnionParam, error) {
	schema, err := toolSchemaMap(tool.InputSchema)
	if err != nil {
		return responses.ToolUnionParam{}, fmt.Errorf("llm: convert OpenAI Responses tool %q schema: %w", tool.Name, err)
	}

	converted := responses.FunctionToolParam{
		Name:        tool.Name,
		Description: openai.String(tool.Description),
		Parameters:  schema,
		Strict:      openai.Bool(tool.Strict),
	}

	return responses.ToolUnionParam{OfFunction: &converted}, nil
}

func openAIResponsesMessage(message conversation.Message) (responses.ResponseInputParam, error) {
	role := responses.EasyInputMessageRole(message.Role)
	content := make(responses.ResponseInputMessageContentListParam, 0, len(message.Content))
	items := make(responses.ResponseInputParam, 0, len(message.Content)+1)

	flush := func() {
		if len(content) == 0 {
			return
		}
		items = append(items, responses.ResponseInputItemParamOfMessage(content, role))
		content = nil
	}

	for _, block := range message.Content {
		switch value := block.(type) {
		case conversation.TextBlock:
			content = append(content, responses.ResponseInputContentParamOfInputText(value.Text))
		case conversation.ImageBlock:
			if message.Role != conversation.RoleUser {
				return nil, unsupportedContent("OpenAI Responses images require a user message")
			}
			url, err := imageURL(value)
			if err != nil {
				return nil, err
			}
			image := responses.ResponseInputContentParamOfInputImage(responses.ResponseInputImageDetailAuto)
			image.OfInputImage.ImageURL = openai.String(url)
			content = append(content, image)
		case conversation.ReasoningBlock:
			if message.Role != conversation.RoleAssistant || len(value.Extra) == 0 {
				return nil, unsupportedContent("OpenAI Responses reasoning requires assistant role and preserved extra data")
			}
			flush()
			var reasoning responses.ResponseReasoningItemParam
			if err := json.Unmarshal(value.Extra, &reasoning); err != nil {
				return nil, invalidRequest("reasoning extra data is invalid: %v", err)
			}
			items = append(items, responses.ResponseInputItemUnionParam{OfReasoning: &reasoning})
		case conversation.ToolUseBlock:
			if message.Role != conversation.RoleAssistant {
				return nil, unsupportedContent("OpenAI Responses tool use requires assistant role")
			}
			flush()
			if _, err := rawObject(value.Input, "tool input"); err != nil {
				return nil, err
			}
			items = append(items, responses.ResponseInputItemParamOfFunctionCall(
				string(value.Input), value.ID, value.Name,
			))
		case conversation.ToolResultBlock:
			flush()
			output, err := textContent(value.Content)
			if err != nil {
				return nil, err
			}
			items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(
				value.ToolUseID, output,
			))
		default:
			return nil, unsupportedContent("unknown OpenAI Responses block %q", block.BlockType())
		}
	}
	flush()

	return items, nil
}

func openAIResponsesResponse(result *responses.Response) (*modelResponse, error) {
	content := make(conversation.Content, 0, len(result.Output))
	hasToolUse := false
	for _, output := range result.Output {
		switch item := output.AsAny().(type) {
		case responses.ResponseOutputMessage:
			for _, part := range item.Content {
				switch value := part.AsAny().(type) {
				case responses.ResponseOutputText:
					content = append(content, conversation.TextBlock{Text: value.Text})
				case responses.ResponseOutputRefusal:
					content = append(content, conversation.TextBlock{Text: value.Refusal})
				}
			}
		case responses.ResponseFunctionToolCall:
			hasToolUse = true
			arguments := shared.RawJSON(item.Arguments)
			if !json.Valid(arguments) {
				return nil, fmt.Errorf("llm: OpenAI Responses returned invalid tool arguments for %q", item.Name)
			}
			content = append(content, conversation.ToolUseBlock{
				ID: item.CallID, Name: item.Name, Input: arguments,
			})
		case responses.ResponseReasoningItem:
			parts := make([]string, 0, len(item.Content)+len(item.Summary))
			for _, part := range item.Content {
				parts = append(parts, part.Text)
			}
			for _, part := range item.Summary {
				parts = append(parts, part.Text)
			}
			content = append(content, conversation.ReasoningBlock{
				Reasoning: strings.Join(parts, ""), Signature: item.ID,
				Extra: shared.RawJSON(item.RawJSON()),
			})
		}
	}

	stopReason := modelStopReasonEndTurn
	if hasToolUse {
		stopReason = modelStopReasonToolUse
	} else if result.Status == responses.ResponseStatusIncomplete && result.IncompleteDetails.Reason == "max_output_tokens" {
		stopReason = modelStopReasonMaxTokens
	} else if result.Status == responses.ResponseStatusFailed {
		stopReason = modelStopReasonError
	}

	return &modelResponse{
		ID: result.ID, Model: string(result.Model), Content: content, StopReason: stopReason,
		Usage: conversation.TokenUsage{
			Input: result.Usage.InputTokens, Output: result.Usage.OutputTokens,
			CachedRead: result.Usage.InputTokensDetails.CachedTokens,
			CacheWrite: result.Usage.InputTokensDetails.CacheWriteTokens,
			Reasoning:  result.Usage.OutputTokensDetails.ReasoningTokens,
			Total:      result.Usage.TotalTokens,
		},
	}, nil
}

type openAIResponsesStreamEvent struct {
	Type        string `json:"type"`
	Delta       string `json:"delta"`
	OutputIndex int    `json:"output_index"`
	Item        struct {
		Type      string `json:"type"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"item"`
}

func emitOpenAIResponsesEvent(handler modelStreamHandler, event openAIResponsesStreamEvent) error {
	switch event.Type {
	case "response.output_text.delta":
		return emit(handler, modelStreamEvent{Type: modelStreamEventTextDelta, Index: event.OutputIndex, Delta: event.Delta})
	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		return emit(handler, modelStreamEvent{Type: modelStreamEventReasoningDelta, Index: event.OutputIndex, Delta: event.Delta})
	case "response.output_item.added":
		if event.Item.Type == "function_call" {
			return emit(handler, modelStreamEvent{
				Type: modelStreamEventToolUseStart, Index: event.OutputIndex,
				ToolUseID: event.Item.CallID, ToolName: event.Item.Name,
			})
		}
	case "response.function_call_arguments.delta":
		return emit(handler, modelStreamEvent{Type: modelStreamEventToolInputDelta, Index: event.OutputIndex, Delta: event.Delta})
	case "response.output_item.done":
		if event.Item.Type == "function_call" {
			return emit(handler, modelStreamEvent{
				Type: modelStreamEventToolUseDone, Index: event.OutputIndex,
				ToolUseID: event.Item.CallID, ToolName: event.Item.Name,
				ToolInput: shared.RawJSON(event.Item.Arguments),
			})
		}
	}

	return nil
}
