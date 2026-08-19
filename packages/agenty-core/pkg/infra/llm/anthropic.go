package llm

import (
	"context"
	"fmt"
	"maps"

	"github.com/anthropics/anthropic-sdk-go"
	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type anthropicCaller struct {
	client *anthropic.Client
	model  catalog.Model
}

func (caller *anthropicCaller) Invoke(ctx context.Context, request modelRequest) (*modelResponse, error) {
	params, err := caller.params(request)
	if err != nil {
		return nil, err
	}

	result, err := caller.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("llm: invoke anthropic messages API: %w", err)
	}

	return anthropicResponse(result)
}

func (caller *anthropicCaller) Stream(
	ctx context.Context,
	request modelRequest,
	handler modelStreamHandler,
) (*modelResponse, error) {
	params, err := caller.params(request)
	if err != nil {
		return nil, err
	}

	stream := caller.client.Messages.NewStreaming(ctx, params)
	defer stream.Close()

	var accumulator anthropic.Message
	for stream.Next() {
		event := stream.Current()
		if err := accumulator.Accumulate(event); err != nil {
			return nil, fmt.Errorf("llm: accumulate anthropic messages stream event: %w", err)
		}
		data, err := json.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("llm: marshal anthropic messages stream event: %w", err)
		}
		var wire anthropicStreamEvent
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, fmt.Errorf("llm: decode anthropic messages stream event: %w", err)
		}
		if err := emitAnthropicEvent(handler, wire); err != nil {
			return nil, err
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("llm: stream anthropic messages API: %w", err)
	}

	final, err := anthropicResponse(&accumulator)
	if err != nil {
		return nil, err
	}
	for index, block := range final.Content {
		tool, ok := block.(conversation.ToolUseBlock)
		if !ok {
			continue
		}
		if err := emit(handler, modelStreamEvent{
			Type:      modelStreamEventToolUseDone,
			Index:     index,
			ToolUseID: tool.ID,
			ToolName:  tool.Name,
			ToolInput: tool.Input,
		}); err != nil {
			return nil, err
		}
	}
	if err := emit(handler, modelStreamEvent{Type: modelStreamEventCompleted, Response: final}); err != nil {
		return nil, err
	}

	return final, nil
}

func (caller *anthropicCaller) params(request modelRequest) (anthropic.MessageNewParams, error) {
	if err := validateRequest(request); err != nil {
		return anthropic.MessageNewParams{}, err
	}

	prompt, err := systemPrompt(request)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}
	effort, err := nativeReasoningEffort(caller.model, request.ReasoningEffort)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}

	messages := make([]anthropic.MessageParam, 0, len(request.Messages))
	for index, message := range request.Messages {
		if message.Role == conversation.RoleSystem {
			continue
		}

		converted, err := anthropicMessage(message)
		if err != nil {
			return anthropic.MessageNewParams{}, fmt.Errorf("llm: convert anthropic message %d: %w", index, err)
		}
		messages = append(messages, converted)
	}

	tools, err := anthropicTools(request.Tools)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(caller.model.Slug.String()),
		Messages:  messages,
		MaxTokens: request.MaxOutputTokens,
		Tools:     tools,
	}
	if prompt != "" {
		params.System = []anthropic.TextBlockParam{{Text: prompt}}
	}
	if effort != "" {
		if request.ReasoningBudgetTokens > 0 {
			params.Thinking = anthropic.ThinkingConfigParamOfEnabled(request.ReasoningBudgetTokens)
		} else {
			params.Thinking = anthropic.ThinkingConfigParamUnion{
				OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
			}
		}
		params.OutputConfig.Effort = anthropic.OutputConfigEffort(effort)
	}

	return params, nil
}

func anthropicTools(definitions []modelToolDefinition) ([]anthropic.ToolUnionParam, error) {
	tools := make([]anthropic.ToolUnionParam, 0, len(definitions))
	for _, definition := range definitions {
		tool, err := anthropicToolDefinition(definition)
		if err != nil {
			return nil, err
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func anthropicToolDefinition(tool modelToolDefinition) (anthropic.ToolUnionParam, error) {
	if _, err := providerToolType(tool); err != nil {
		return anthropic.ToolUnionParam{}, err
	}
	schema, err := toolSchemaMap(tool.InputSchema)
	if err != nil {
		return anthropic.ToolUnionParam{}, fmt.Errorf("llm: convert Anthropic tool %q schema: %w", tool.Name, err)
	}

	extraFields := make(map[string]any, len(schema))
	maps.Copy(extraFields, schema)
	delete(extraFields, "type")
	delete(extraFields, "properties")
	delete(extraFields, "required")

	inputSchema := anthropic.ToolInputSchemaParam{
		Properties:  schema["properties"],
		Required:    tool.InputSchema.Required,
		ExtraFields: extraFields,
	}
	converted := anthropic.ToolUnionParamOfTool(inputSchema, tool.Name)
	converted.OfTool.Description = anthropic.String(tool.Description)
	converted.OfTool.Strict = anthropic.Bool(tool.Strict)

	return converted, nil
}

func anthropicMessage(message conversation.Message) (anthropic.MessageParam, error) {
	content := make([]anthropic.ContentBlockParamUnion, 0, len(message.Content))
	for _, block := range message.Content {
		switch value := block.(type) {
		case conversation.TextBlock:
			content = append(content, anthropic.NewTextBlock(value.Text))
		case conversation.ImageBlock:
			if message.Role != conversation.RoleUser {
				return anthropic.MessageParam{}, unsupportedContent("Anthropic images require a user message")
			}
			if value.Data != "" {
				if _, err := imageURL(value); err != nil {
					return anthropic.MessageParam{}, err
				}
				content = append(content, anthropic.NewImageBlockBase64(value.MimeType, value.Data))
			} else {
				url, err := imageURL(value)
				if err != nil {
					return anthropic.MessageParam{}, err
				}
				content = append(content, anthropic.NewImageBlock(anthropic.URLImageSourceParam{URL: url}))
			}
		case conversation.ReasoningBlock:
			if message.Role != conversation.RoleAssistant {
				return anthropic.MessageParam{}, unsupportedContent("Anthropic thinking requires assistant role")
			}
			if value.Redacted {
				content = append(content, anthropic.NewRedactedThinkingBlock(value.Signature))
			} else {
				content = append(content, anthropic.NewThinkingBlock(value.Signature, value.Reasoning))
			}
		case conversation.ToolUseBlock:
			if message.Role != conversation.RoleAssistant {
				return anthropic.MessageParam{}, unsupportedContent("Anthropic tool use requires assistant role")
			}
			if _, err := rawObject(value.Input, "tool input"); err != nil {
				return anthropic.MessageParam{}, err
			}
			content = append(content, anthropic.NewToolUseBlock(value.ID, value.Input, value.Name))
		case conversation.ShellCallBlock:
			if message.Role != conversation.RoleAssistant {
				return anthropic.MessageParam{}, unsupportedContent("Anthropic shell call requires assistant role")
			}
			call := value.ToolUseBlock()
			content = append(content, anthropic.NewToolUseBlock(call.ID, call.Input, call.Name))
		case conversation.ToolResultBlock:
			if message.Role != conversation.RoleUser {
				return anthropic.MessageParam{}, unsupportedContent("Anthropic tool result requires user role")
			}
			resultContent, err := anthropicToolResultContent(value.Content)
			if err != nil {
				return anthropic.MessageParam{}, err
			}
			content = append(content, anthropic.ContentBlockParamUnion{
				OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: value.ToolUseID,
					Content:   resultContent,
					IsError:   anthropic.Bool(value.IsError),
				},
			})
		default:
			return anthropic.MessageParam{}, unsupportedContent("unknown Anthropic block %q", block.BlockType())
		}
	}

	if message.Role == conversation.RoleAssistant {
		return anthropic.NewAssistantMessage(content...), nil
	}
	return anthropic.NewUserMessage(content...), nil
}

func anthropicToolResultContent(content conversation.Content) ([]anthropic.ToolResultBlockParamContentUnion, error) {
	result := make([]anthropic.ToolResultBlockParamContentUnion, 0, len(content))
	for _, block := range content {
		switch value := block.(type) {
		case conversation.TextBlock:
			result = append(result, anthropic.ToolResultBlockParamContentUnion{
				OfText: &anthropic.TextBlockParam{Text: value.Text},
			})
		case conversation.ShellCallOutputBlock:
			text, err := textContent(conversation.Content{value})
			if err != nil {
				return nil, err
			}
			result = append(result, anthropic.ToolResultBlockParamContentUnion{
				OfText: &anthropic.TextBlockParam{Text: text},
			})
		case conversation.ImageBlock:
			if value.Data == "" {
				return nil, unsupportedContent("Anthropic tool-result images must use inline data")
			}
			if _, err := imageURL(value); err != nil {
				return nil, err
			}
			image := anthropic.NewImageBlockBase64(value.MimeType, value.Data)
			result = append(result, anthropic.ToolResultBlockParamContentUnion{OfImage: image.OfImage})
		default:
			return nil, unsupportedContent("Anthropic tool result cannot contain %q", block.BlockType())
		}
	}

	return result, nil
}

func anthropicResponse(result *anthropic.Message) (*modelResponse, error) {
	content := make(conversation.Content, 0, len(result.Content))
	for _, item := range result.Content {
		switch item.Type {
		case "text":
			content = append(content, conversation.TextBlock{Text: item.Text})
		case "thinking":
			extra, err := rawJSON(item)
			if err != nil {
				return nil, err
			}
			content = append(content, conversation.ReasoningBlock{
				Reasoning: item.Thinking, Signature: item.Signature, Extra: extra,
			})
		case "redacted_thinking":
			extra, err := rawJSON(item)
			if err != nil {
				return nil, err
			}
			content = append(content, conversation.ReasoningBlock{
				Signature: item.Data, Redacted: true, Extra: extra,
			})
		case "tool_use":
			if !json.Valid(item.Input) {
				return nil, fmt.Errorf("llm: Anthropic returned invalid tool input for %q", item.Name)
			}
			content = append(content, conversation.ToolUseBlock{
				ID: item.ID, Name: item.Name, Input: shared.RawJSON(item.Input),
			})
		}
	}

	usage := conversation.TokenUsage{
		Input: result.Usage.InputTokens, Output: result.Usage.OutputTokens,
		CachedRead: result.Usage.CacheReadInputTokens,
		CacheWrite: result.Usage.CacheCreationInputTokens,
	}
	usage.Total = usage.Input + usage.Output

	return &modelResponse{
		ID: result.ID, Model: string(result.Model), Content: content,
		Usage: usage, StopReason: anthropicStopReason(string(result.StopReason)),
	}, nil
}

func anthropicStopReason(reason string) modelStopReason {
	switch reason {
	case "max_tokens", "model_context_window_exceeded":
		return modelStopReasonMaxTokens
	case "tool_use":
		return modelStopReasonToolUse
	case "refusal":
		return modelStopReasonContentFilter
	default:
		return modelStopReasonEndTurn
	}
}

type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
}

func emitAnthropicEvent(handler modelStreamHandler, event anthropicStreamEvent) error {
	switch event.Type {
	case "content_block_start":
		if event.ContentBlock.Type == "tool_use" {
			return emit(handler, modelStreamEvent{
				Type: modelStreamEventToolUseStart, Index: event.Index,
				ToolUseID: event.ContentBlock.ID, ToolName: event.ContentBlock.Name,
			})
		}
	case "content_block_delta":
		switch event.Delta.Type {
		case "text_delta":
			return emit(handler, modelStreamEvent{Type: modelStreamEventTextDelta, Index: event.Index, Delta: event.Delta.Text})
		case "thinking_delta":
			return emit(handler, modelStreamEvent{Type: modelStreamEventReasoningDelta, Index: event.Index, Delta: event.Delta.Thinking})
		case "input_json_delta":
			return emit(handler, modelStreamEvent{Type: modelStreamEventToolInputDelta, Index: event.Index, Delta: event.Delta.PartialJSON})
		}
	}

	return nil
}
