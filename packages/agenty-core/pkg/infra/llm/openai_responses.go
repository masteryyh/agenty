package llm

import (
	"context"
	"fmt"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	openaishared "github.com/openai/openai-go/v3/shared"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type openAIResponsesCaller struct {
	client       *openai.Client
	model        catalog.Model
	nativeOpenAI bool
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
			switch item := event.Item.AsAny().(type) {
			case responses.ResponseFunctionToolCall:
				err = emit(handler, modelStreamEvent{
					Type: modelStreamEventToolUseStart, Index: int(event.OutputIndex),
					ToolUseID: item.CallID, ToolName: item.Name,
				})
			case responses.ResponseFunctionShellToolCall:
				err = emit(handler, modelStreamEvent{
					Type: modelStreamEventToolUseStart, Index: int(event.OutputIndex),
					ToolUseID: item.CallID, ToolName: "shell",
				})
			case responses.ResponseApplyPatchToolCall:
				err = emit(handler, modelStreamEvent{
					Type: modelStreamEventToolUseStart, Index: int(event.OutputIndex),
					ToolUseID: item.CallID, ToolName: "apply_patch",
				})
			case responses.ResponseCustomToolCall:
				if item.Name == "apply_patch" {
					err = emit(handler, modelStreamEvent{
						Type: modelStreamEventToolUseStart, Index: int(event.OutputIndex),
						ToolUseID: item.CallID, ToolName: item.Name,
					})
				}
			}
		case responses.ResponseOutputItemDoneEvent:
			switch item := event.Item.AsAny().(type) {
			case responses.ResponseFunctionToolCall:
				err = emit(handler, modelStreamEvent{
					Type: modelStreamEventToolUseDone, Index: int(event.OutputIndex),
					ToolUseID: item.CallID, ToolName: item.Name,
					ToolInput: shared.RawJSON(item.Arguments),
				})
			case responses.ResponseFunctionShellToolCall:
				input := conversation.ShellCallBlock{
					CallID: item.CallID, Commands: item.Action.Commands,
					TimeoutMs: item.Action.TimeoutMs, MaxOutputLength: item.Action.MaxOutputLength,
				}.ToolUseBlock().Input
				err = emit(handler, modelStreamEvent{
					Type: modelStreamEventToolUseDone, Index: int(event.OutputIndex),
					ToolUseID: item.CallID, ToolName: "shell", ToolInput: input,
				})
			case responses.ResponseApplyPatchToolCall:
				operation, operationErr := openAIApplyPatchOperation(item.Operation)
				if operationErr != nil {
					err = operationErr
					break
				}
				input := conversation.ApplyPatchCallBlock{
					CallID: item.CallID, Source: conversation.ApplyPatchSourceNative,
					Operation: &operation,
				}.ToolUseBlock().Input
				err = emit(handler, modelStreamEvent{
					Type: modelStreamEventToolUseDone, Index: int(event.OutputIndex),
					ToolUseID: item.CallID, ToolName: "apply_patch", ToolInput: input,
				})
			case responses.ResponseCustomToolCall:
				if item.Name == "apply_patch" {
					input := conversation.ApplyPatchCallBlock{
						CallID: item.CallID, Source: conversation.ApplyPatchSourceCustom,
						Patch: item.Input,
					}.ToolUseBlock().Input
					err = emit(handler, modelStreamEvent{
						Type: modelStreamEventToolUseDone, Index: int(event.OutputIndex),
						ToolUseID: item.CallID, ToolName: item.Name, ToolInput: input,
					})
				}
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

	input, err := openAIResponsesMessages(request.Messages, caller.nativeOpenAI)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}

	tools, err := openAIResponsesTools(request.Tools, caller.nativeOpenAI)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}

	params := responses.ResponseNewParams{
		Model:           openaishared.ResponsesModel(caller.model.Code.String()),
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

func openAIResponsesTools(definitions []modelToolDefinition, nativeOpenAI bool) ([]responses.ToolUnionParam, error) {
	tools := make([]responses.ToolUnionParam, 0, len(definitions))
	for _, definition := range definitions {
		if isReplacedFileTool(definition.Name) {
			continue
		}
		tool, err := openAIResponsesToolDefinition(definition, nativeOpenAI)
		if err != nil {
			return nil, err
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func openAIResponsesMessages(
	messages []conversation.Message,
	nativeOpenAI bool,
) (responses.ResponseInputParam, error) {
	callSources := openAIResponsesCallSources(messages)
	input := make(responses.ResponseInputParam, 0, len(messages))
	for index, message := range messages {
		if message.Role == conversation.RoleSystem {
			continue
		}
		items, err := openAIResponsesMessageWithNativeCallIDs(
			message,
			nativeOpenAI,
			callSources,
		)
		if err != nil {
			return nil, fmt.Errorf("llm: convert OpenAI Responses message %d: %w", index, err)
		}
		input = append(input, items...)
	}
	return input, nil
}

type openAIResponsesCallSource struct {
	nativeShell      bool
	applyPatchSource conversation.ApplyPatchCallSource
}

func openAIResponsesCallSources(messages []conversation.Message) map[string]openAIResponsesCallSource {
	sources := make(map[string]openAIResponsesCallSource)
	for _, message := range messages {
		for _, block := range message.Content {
			switch call := block.(type) {
			case conversation.ShellCallBlock:
				if call.CallID != "" {
					sources[call.CallID] = openAIResponsesCallSource{nativeShell: true}
				}
			case conversation.ApplyPatchCallBlock:
				if call.CallID != "" {
					sources[call.CallID] = openAIResponsesCallSource{
						applyPatchSource: call.Source,
					}
				}
			}
		}
	}
	return sources
}

func isReplacedFileTool(name string) bool {
	switch name {
	case "write_file", "patch_file", "delete_file":
		return true
	default:
		return false
	}
}

func openAIResponsesToolDefinition(tool modelToolDefinition, nativeOpenAI bool) (responses.ToolUnionParam, error) {
	toolType, err := providerToolType(tool)
	if err != nil {
		return responses.ToolUnionParam{}, err
	}
	if toolType == agentloop.ToolTypeShell && nativeOpenAI {
		return responses.ToolUnionParam{OfShell: &responses.FunctionShellToolParam{
			Environment: responses.FunctionShellToolEnvironmentUnionParam{
				OfLocal: &responses.LocalEnvironmentParam{},
			},
		}}, nil
	}
	if toolType == agentloop.ToolTypeApplyPatch {
		if nativeOpenAI {
			return responses.ToolUnionParam{OfApplyPatch: &responses.ApplyPatchToolParam{}}, nil
		}
		return responses.ToolUnionParam{OfCustom: &responses.CustomToolParam{
			Name:        "apply_patch",
			Description: openai.String(tool.Description),
		}}, nil
	}

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

func openAIResponsesMessage(message conversation.Message, nativeOpenAI bool) (responses.ResponseInputParam, error) {
	return openAIResponsesMessageWithNativeCallIDs(message, nativeOpenAI, nil)
}

func openAIResponsesMessageWithNativeCallIDs(
	message conversation.Message,
	nativeOpenAI bool,
	callSources map[string]openAIResponsesCallSource,
) (responses.ResponseInputParam, error) {
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
		case conversation.ShellCallBlock:
			if message.Role != conversation.RoleAssistant {
				return nil, unsupportedContent("OpenAI Responses shell call requires assistant role")
			}
			flush()
			if !nativeOpenAI {
				call := value.ToolUseBlock()
				if _, err := rawObject(call.Input, "tool input"); err != nil {
					return nil, err
				}
				items = append(items, responses.ResponseInputItemParamOfFunctionCall(
					string(call.Input), call.ID, call.Name,
				))
				continue
			}
			action := responses.ResponseInputItemShellCallActionParam{Commands: value.Commands}
			if value.TimeoutMs > 0 {
				action.TimeoutMs = openai.Int(value.TimeoutMs)
			}
			if value.MaxOutputLength > 0 {
				action.MaxOutputLength = openai.Int(value.MaxOutputLength)
			}
			item := responses.ResponseInputItemParamOfShellCall(action, value.CallID)
			if value.ID != "" {
				item.OfShellCall.ID = openai.String(value.ID)
			}
			item.OfShellCall.Environment.OfLocal = &responses.LocalEnvironmentParam{}
			items = append(items, item)
		case conversation.ApplyPatchCallBlock:
			if message.Role != conversation.RoleAssistant {
				return nil, unsupportedContent("OpenAI Responses apply patch call requires assistant role")
			}
			flush()
			item, err := openAIResponsesApplyPatchCall(value, nativeOpenAI)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		case conversation.ToolResultBlock:
			flush()
			if source := callSources[value.ToolUseID].applyPatchSource; source != "" {
				output, err := textContent(value.Content)
				if err != nil {
					return nil, err
				}
				if source == conversation.ApplyPatchSourceNative && nativeOpenAI {
					status := "completed"
					if value.IsError {
						status = "failed"
					}
					item := responses.ResponseInputItemParamOfApplyPatchCallOutput(
						value.ToolUseID,
						status,
					)
					if output != "" {
						item.OfApplyPatchCallOutput.Output = openai.String(output)
					}
					items = append(items, item)
				} else {
					items = append(items, responses.ResponseInputItemParamOfCustomToolCallOutput(
						value.ToolUseID,
						output,
					))
				}
				continue
			}
			useNativeShellOutput := false
			if output, ok := shellCallOutput(value.Content); ok {
				if output.OpenAINative != nil {
					useNativeShellOutput = nativeOpenAI && *output.OpenAINative
				} else {
					useNativeShellOutput = nativeOpenAI && callSources[value.ToolUseID].nativeShell
				}
			}
			if useNativeShellOutput {
				if output, ok := shellCallOutput(value.Content); ok {
					item, err := openAIResponsesShellCallOutput(output)
					if err != nil {
						return nil, err
					}
					items = append(items, item)
					continue
				}
			}
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

func shellCallOutput(content conversation.Content) (conversation.ShellCallOutputBlock, bool) {
	if len(content) != 1 {
		return conversation.ShellCallOutputBlock{}, false
	}
	output, ok := content[0].(conversation.ShellCallOutputBlock)
	return output, ok
}

func openAIResponsesApplyPatchCall(
	call conversation.ApplyPatchCallBlock,
	nativeOpenAI bool,
) (responses.ResponseInputItemUnionParam, error) {
	if call.CallID == "" {
		return responses.ResponseInputItemUnionParam{}, invalidRequest("apply patch call ID is required")
	}

	if call.Source == conversation.ApplyPatchSourceCustom {
		if call.Patch == "" {
			return responses.ResponseInputItemUnionParam{}, invalidRequest("custom apply patch call has no patch")
		}
		item := responses.ResponseInputItemParamOfCustomToolCall(call.CallID, call.Patch, "apply_patch")
		if call.ID != "" {
			item.OfCustomToolCall.ID = openai.String(call.ID)
		}
		return item, nil
	}
	if call.Source != conversation.ApplyPatchSourceNative || call.Operation == nil {
		return responses.ResponseInputItemUnionParam{}, invalidRequest("native apply patch call has no operation")
	}
	if !nativeOpenAI {
		patch, err := applyPatchOperationEnvelope(*call.Operation)
		if err != nil {
			return responses.ResponseInputItemUnionParam{}, err
		}
		item := responses.ResponseInputItemParamOfCustomToolCall(call.CallID, patch, "apply_patch")
		if call.ID != "" {
			item.OfCustomToolCall.ID = openai.String(call.ID)
		}
		return item, nil
	}

	operation := *call.Operation
	var item responses.ResponseInputItemUnionParam
	switch operation.Type {
	case conversation.ApplyPatchCreateFile:
		item = responses.ResponseInputItemParamOfApplyPatchCall(
			call.CallID,
			responses.ResponseInputItemApplyPatchCallOperationCreateFileParam{
				Path: operation.Path,
				Diff: operation.Diff,
			},
			"completed",
		)
	case conversation.ApplyPatchDeleteFile:
		item = responses.ResponseInputItemParamOfApplyPatchCall(
			call.CallID,
			responses.ResponseInputItemApplyPatchCallOperationDeleteFileParam{Path: operation.Path},
			"completed",
		)
	case conversation.ApplyPatchUpdateFile:
		if operation.MoveTo != "" {
			return responses.ResponseInputItemUnionParam{}, invalidRequest(
				"native apply patch operation cannot move files",
			)
		}
		item = responses.ResponseInputItemParamOfApplyPatchCall(
			call.CallID,
			responses.ResponseInputItemApplyPatchCallOperationUpdateFileParam{
				Path: operation.Path,
				Diff: operation.Diff,
			},
			"completed",
		)
	default:
		return responses.ResponseInputItemUnionParam{}, invalidRequest(
			"apply patch operation has unknown type %q",
			operation.Type,
		)
	}
	if call.ID != "" {
		item.OfApplyPatchCall.ID = openai.String(call.ID)
	}
	return item, nil
}

func applyPatchOperationEnvelope(operation conversation.ApplyPatchOperation) (string, error) {
	var header string
	switch operation.Type {
	case conversation.ApplyPatchCreateFile:
		header = "*** Add File: " + operation.Path
	case conversation.ApplyPatchDeleteFile:
		header = "*** Delete File: " + operation.Path
	case conversation.ApplyPatchUpdateFile:
		header = "*** Update File: " + operation.Path
	default:
		return "", invalidRequest("apply patch operation has unknown type %q", operation.Type)
	}
	if operation.Path == "" {
		return "", invalidRequest("apply patch operation path is required")
	}

	var patch strings.Builder
	patch.WriteString("*** Begin Patch\n")
	patch.WriteString(header)
	patch.WriteByte('\n')
	if operation.MoveTo != "" {
		if operation.Type != conversation.ApplyPatchUpdateFile {
			return "", invalidRequest("only update operations can move files")
		}
		patch.WriteString("*** Move to: ")
		patch.WriteString(operation.MoveTo)
		patch.WriteByte('\n')
	}
	patch.WriteString(operation.Diff)
	if operation.Diff != "" && !strings.HasSuffix(operation.Diff, "\n") {
		patch.WriteByte('\n')
	}
	patch.WriteString("*** End Patch")
	return patch.String(), nil
}

func openAIApplyPatchOperation(
	operation responses.ResponseApplyPatchToolCallOperationUnion,
) (conversation.ApplyPatchOperation, error) {
	converted := conversation.ApplyPatchOperation{
		Type: conversation.ApplyPatchOperationType(operation.Type),
		Path: operation.Path,
		Diff: operation.Diff,
	}
	if converted.Path == "" {
		return conversation.ApplyPatchOperation{}, invalidRequest("apply patch operation path is required")
	}
	switch converted.Type {
	case conversation.ApplyPatchCreateFile, conversation.ApplyPatchUpdateFile:
	case conversation.ApplyPatchDeleteFile:
		converted.Diff = ""
	default:
		return conversation.ApplyPatchOperation{}, invalidRequest(
			"apply patch operation has unknown type %q",
			operation.Type,
		)
	}
	return converted, nil
}

func openAIResponsesShellCallOutput(
	output conversation.ShellCallOutputBlock,
) (responses.ResponseInputItemUnionParam, error) {
	contents := make([]responses.ResponseFunctionShellCallOutputContentParam, 0, len(output.Output))
	for index, command := range output.Output {
		converted := responses.ResponseFunctionShellCallOutputContentParam{
			Stdout: command.Stdout,
			Stderr: command.Stderr,
		}
		switch command.Outcome.Type {
		case "timeout":
			timeout := responses.NewResponseFunctionShellCallOutputContentOutcomeTimeoutParam()
			converted.Outcome.OfTimeout = &timeout
		case "exit":
			if command.Outcome.ExitCode == nil {
				return responses.ResponseInputItemUnionParam{}, invalidRequest(
					"shell output %d exit outcome has no exit code", index,
				)
			}
			converted.Outcome.OfExit = &responses.ResponseFunctionShellCallOutputContentOutcomeExitParam{
				ExitCode: *command.Outcome.ExitCode,
			}
		default:
			return responses.ResponseInputItemUnionParam{}, invalidRequest(
				"shell output %d has unknown outcome %q", index, command.Outcome.Type,
			)
		}
		contents = append(contents, converted)
	}

	item := responses.ResponseInputItemParamOfShellCallOutput(output.CallID, contents)
	if output.MaxOutputLength > 0 {
		item.OfShellCallOutput.MaxOutputLength = openai.Int(output.MaxOutputLength)
	}
	return item, nil
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
		case responses.ResponseFunctionShellToolCall:
			hasToolUse = true
			content = append(content, conversation.ShellCallBlock{
				ID:              item.ID,
				CallID:          item.CallID,
				Commands:        item.Action.Commands,
				TimeoutMs:       item.Action.TimeoutMs,
				MaxOutputLength: item.Action.MaxOutputLength,
			})
		case responses.ResponseApplyPatchToolCall:
			hasToolUse = true
			operation, err := openAIApplyPatchOperation(item.Operation)
			if err != nil {
				return nil, fmt.Errorf("llm: OpenAI Responses returned invalid apply patch operation: %w", err)
			}
			content = append(content, conversation.ApplyPatchCallBlock{
				ID: item.ID, CallID: item.CallID, Source: conversation.ApplyPatchSourceNative,
				Operation: &operation,
			})
		case responses.ResponseCustomToolCall:
			if item.Name != "apply_patch" {
				continue
			}
			hasToolUse = true
			if item.Input == "" {
				return nil, fmt.Errorf("llm: OpenAI Responses returned an empty custom apply patch input")
			}
			content = append(content, conversation.ApplyPatchCallBlock{
				ID: item.ID, CallID: item.CallID, Source: conversation.ApplyPatchSourceCustom,
				Patch: item.Input,
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
		Input     string `json:"input"`
		Operation struct {
			Type string `json:"type"`
			Path string `json:"path"`
			Diff string `json:"diff"`
		} `json:"operation"`
	} `json:"item"`
}

func emitOpenAIResponsesEvent(handler modelStreamHandler, event openAIResponsesStreamEvent) error {
	switch event.Type {
	case "response.output_text.delta":
		return emit(handler, modelStreamEvent{Type: modelStreamEventTextDelta, Index: event.OutputIndex, Delta: event.Delta})
	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		return emit(handler, modelStreamEvent{Type: modelStreamEventReasoningDelta, Index: event.OutputIndex, Delta: event.Delta})
	case "response.output_item.added":
		if event.Item.Type == "function_call" ||
			event.Item.Type == "apply_patch_call" ||
			(event.Item.Type == "custom_tool_call" && event.Item.Name == "apply_patch") {
			name := event.Item.Name
			if event.Item.Type == "apply_patch_call" {
				name = "apply_patch"
			}
			return emit(handler, modelStreamEvent{
				Type: modelStreamEventToolUseStart, Index: event.OutputIndex,
				ToolUseID: event.Item.CallID, ToolName: name,
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
		if event.Item.Type == "apply_patch_call" {
			operation := conversation.ApplyPatchOperation{
				Type: conversation.ApplyPatchOperationType(event.Item.Operation.Type),
				Path: event.Item.Operation.Path,
				Diff: event.Item.Operation.Diff,
			}
			input := conversation.ApplyPatchCallBlock{
				CallID: event.Item.CallID, Source: conversation.ApplyPatchSourceNative,
				Operation: &operation,
			}.ToolUseBlock().Input
			return emit(handler, modelStreamEvent{
				Type: modelStreamEventToolUseDone, Index: event.OutputIndex,
				ToolUseID: event.Item.CallID, ToolName: "apply_patch", ToolInput: input,
			})
		}
		if event.Item.Type == "custom_tool_call" && event.Item.Name == "apply_patch" {
			input := conversation.ApplyPatchCallBlock{
				CallID: event.Item.CallID, Source: conversation.ApplyPatchSourceCustom,
				Patch: event.Item.Input,
			}.ToolUseBlock().Input
			return emit(handler, modelStreamEvent{
				Type: modelStreamEventToolUseDone, Index: event.OutputIndex,
				ToolUseID: event.Item.CallID, ToolName: "apply_patch", ToolInput: input,
			})
		}
	}

	return nil
}
