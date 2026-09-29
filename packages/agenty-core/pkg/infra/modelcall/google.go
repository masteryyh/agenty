package modelcall

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"strings"

	"google.golang.org/genai"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

type googleCaller struct {
	client *genai.Client
	model  ModelCallConfig
}

func (caller *googleCaller) Invoke(ctx context.Context, request ModelCallRequest) (*ModelCallResponse, error) {
	contents, config, err := caller.params(request)
	if err != nil {
		return nil, err
	}

	result, err := caller.client.Models.GenerateContent(ctx, caller.model.ModelCode, contents, config)
	if err != nil {
		return nil, fmt.Errorf("modelcall: invoke Google GenAI SDK: %w", err)
	}

	return googleResponse(result)
}

func (caller *googleCaller) Stream(
	ctx context.Context,
	request ModelCallRequest,
	handler ModelCallStreamHandler,
) (*ModelCallResponse, error) {
	contents, config, err := caller.params(request)
	if err != nil {
		return nil, err
	}

	merged := &genai.GenerateContentResponse{}
	for chunk, streamErr := range caller.client.Models.GenerateContentStream(
		ctx,
		caller.model.ModelCode,
		contents,
		config,
	) {
		if streamErr != nil {
			return nil, fmt.Errorf("modelcall: stream Google GenAI SDK: %w", streamErr)
		}
		if err := emitGoogleChunk(handler, chunk); err != nil {
			return nil, err
		}
		mergeGoogleChunk(merged, chunk)
	}
	if len(merged.Candidates) == 0 {
		return nil, fmt.Errorf("modelcall: Google GenAI stream ended without candidates")
	}

	final, err := googleResponse(merged)
	if err != nil {
		return nil, err
	}
	if err := emit(handler, ModelCallStreamEvent{Type: ModelCallStreamEventCompleted, Response: final}); err != nil {
		return nil, err
	}

	return final, nil
}

func (caller *googleCaller) params(request ModelCallRequest) ([]*genai.Content, *genai.GenerateContentConfig, error) {
	if err := validateRequest(request); err != nil {
		return nil, nil, err
	}
	if request.MaxOutputTokens > math.MaxInt32 {
		return nil, nil, invalidRequest("Google max output tokens must not exceed %d", math.MaxInt32)
	}
	if request.ReasoningBudgetTokens > math.MaxInt32 {
		return nil, nil, invalidRequest("Google reasoning budget tokens must not exceed %d", math.MaxInt32)
	}

	prompt, err := systemPrompt(request)
	if err != nil {
		return nil, nil, err
	}
	effort, err := modelReasoningEffort(caller.model, request.ReasoningEffort)
	if err != nil {
		return nil, nil, err
	}

	toolNames := googleToolNames(request.Messages)
	contents := make([]*genai.Content, 0, len(request.Messages))
	for index, message := range request.Messages {
		if message.Role == conversation.RoleSystem {
			continue
		}
		content, err := googleMessage(message, toolNames)
		if err != nil {
			return nil, nil, fmt.Errorf("modelcall: convert Google message %d: %w", index, err)
		}
		contents = append(contents, content)
	}

	config := &genai.GenerateContentConfig{MaxOutputTokens: int32(request.MaxOutputTokens)}
	if prompt != "" {
		config.SystemInstruction = genai.NewContentFromText(prompt, genai.RoleUser)
	}
	if len(request.Tools) > 0 {
		declarations, err := googleTools(request.Tools)
		if err != nil {
			return nil, nil, err
		}
		config.Tools = []*genai.Tool{{FunctionDeclarations: declarations}}
	}
	if effort != "" {
		thinking := &genai.ThinkingConfig{IncludeThoughts: true}
		if request.ReasoningBudgetTokens > 0 {
			budget := int32(request.ReasoningBudgetTokens)
			thinking.ThinkingBudget = &budget
		} else {
			thinking.ThinkingLevel = googleThinkingLevel(effort)
		}
		config.ThinkingConfig = thinking
	}

	if request.OutputFormat != nil {
		schema, err := request.OutputFormat.Schema.toMap()
		if err != nil {
			return nil, nil, err
		}
		config.ResponseMIMEType = "application/json"
		config.ResponseJsonSchema = schema
	}

	return contents, config, nil
}

func googleTools(definitions []ToolDefinition) ([]*genai.FunctionDeclaration, error) {
	tools := make([]*genai.FunctionDeclaration, 0, len(definitions))
	for _, definition := range definitions {
		if definition.Type == ToolTypeApplyPatch {
			continue
		}
		tool, err := googleToolDefinition(definition)
		if err != nil {
			return nil, err
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func googleToolDefinition(tool ToolDefinition) (*genai.FunctionDeclaration, error) {
	if _, err := providerToolType(tool); err != nil {
		return nil, err
	}
	schema, err := toolSchemaMap(tool.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("modelcall: convert Google tool %q schema: %w", tool.Name, err)
	}

	return &genai.FunctionDeclaration{
		Name:                 tool.Name,
		Description:          tool.Description,
		ParametersJsonSchema: schema,
	}, nil
}

func googleToolNames(messages []ModelCallMessage) map[string]string {
	names := make(map[string]string)
	for _, message := range messages {
		for _, block := range message.Content {
			switch tool := block.(type) {
			case conversation.ToolUseBlock:
				names[tool.ID] = tool.Name
			case conversation.ShellCallBlock:
				names[tool.CallID] = "shell"
			case conversation.ApplyPatchCallBlock:
				names[tool.CallID] = "apply_patch"
			}
		}
	}

	return names
}

func googleMessage(message ModelCallMessage, toolNames map[string]string) (*genai.Content, error) {
	var role genai.Role = genai.RoleUser
	if message.Role == conversation.RoleAssistant {
		role = genai.RoleModel
	}

	parts := make([]*genai.Part, 0, len(message.Content))
	for _, block := range message.Content {
		switch value := block.(type) {
		case conversation.TextBlock:
			parts = append(parts, genai.NewPartFromText(value.Text))
		case conversation.ImageBlock:
			if message.Role != conversation.RoleUser && message.Role != conversation.RoleDeveloper {
				return nil, unsupportedContent("Google images require a user message")
			}
			if value.Data != "" {
				data, err := base64.StdEncoding.DecodeString(value.Data)
				if err != nil {
					return nil, invalidRequest("inline image data is not valid base64: %v", err)
				}
				parts = append(parts, genai.NewPartFromBytes(data, value.MimeType))
			} else {
				if _, err := imageURL(value); err != nil {
					return nil, err
				}
				parts = append(parts, genai.NewPartFromURI(value.URI, value.MimeType))
			}
		case conversation.ReasoningBlock:
			if message.Role != conversation.RoleAssistant {
				return nil, unsupportedContent("Google thought requires assistant role")
			}
			part := &genai.Part{Text: value.Reasoning, Thought: true}
			if value.Signature != "" {
				signature, err := base64.StdEncoding.DecodeString(value.Signature)
				if err != nil {
					return nil, invalidRequest("Google thought signature is not valid base64: %v", err)
				}
				part.ThoughtSignature = signature
			}
			parts = append(parts, part)
		case conversation.ToolUseBlock:
			if message.Role != conversation.RoleAssistant {
				return nil, unsupportedContent("Google function call requires assistant role")
			}
			input, err := rawObject(value.Input, "tool input")
			if err != nil {
				return nil, err
			}
			part := genai.NewPartFromFunctionCall(value.Name, input)
			part.FunctionCall.ID = value.ID
			parts = append(parts, part)
		case conversation.ShellCallBlock:
			if message.Role != conversation.RoleAssistant {
				return nil, unsupportedContent("Google shell call requires assistant role")
			}
			call := value.ToolUseBlock()
			input, err := rawObject(call.Input, "tool input")
			if err != nil {
				return nil, err
			}
			part := genai.NewPartFromFunctionCall(call.Name, input)
			part.FunctionCall.ID = call.ID
			parts = append(parts, part)
		case conversation.ApplyPatchCallBlock:
			if message.Role != conversation.RoleAssistant {
				return nil, unsupportedContent("Google apply patch call requires assistant role")
			}
			call := value.ToolUseBlock()
			input, err := rawObject(call.Input, "tool input")
			if err != nil {
				return nil, err
			}
			part := genai.NewPartFromFunctionCall(call.Name, input)
			part.FunctionCall.ID = call.ID
			parts = append(parts, part)
		case conversation.ToolResultBlock:
			if message.Role != conversation.RoleUser && message.Role != conversation.RoleDeveloper {
				return nil, unsupportedContent("Google function response requires user role")
			}
			name := toolNames[value.ToolUseID]
			if name == "" {
				return nil, invalidRequest("Google tool result %q has no matching tool call", value.ToolUseID)
			}
			key := "output"
			if value.IsError {
				key = "error"
			}
			response, responseParts, err := googleToolResult(value.Content, key)
			if err != nil {
				return nil, err
			}
			part := genai.NewPartFromFunctionResponseWithParts(name, response, responseParts)
			part.FunctionResponse.ID = value.ToolUseID
			parts = append(parts, part)
		default:
			return nil, unsupportedContent("unknown Google block %q", block.BlockType())
		}
	}

	return genai.NewContentFromParts(parts, role), nil
}

func googleToolResult(content conversation.Content, key string) (map[string]any, []*genai.FunctionResponsePart, error) {
	if len(content) == 1 {
		if shellOutput, ok := content[0].(conversation.ShellCallOutputBlock); ok {
			object, err := shellCallOutputObject(shellOutput)
			return object, nil, err
		}
	}

	var text strings.Builder
	var responseParts []*genai.FunctionResponsePart
	for _, block := range content {
		switch value := block.(type) {
		case conversation.TextBlock:
			text.WriteString(value.Text)
		case conversation.ShellCallOutputBlock:
			encoded, err := marshalShellCallOutput(value)
			if err != nil {
				return nil, nil, fmt.Errorf("encode shell output: %w", err)
			}
			text.WriteString(encoded)
		case conversation.ImageBlock:
			if _, err := imageURL(value); err != nil {
				return nil, nil, err
			}
			if value.Data != "" {
				data, err := base64.StdEncoding.DecodeString(value.Data)
				if err != nil {
					return nil, nil, invalidRequest("inline image data is not valid base64: %v", err)
				}
				responseParts = append(responseParts, genai.NewFunctionResponsePartFromBytes(data, value.MimeType))
			} else {
				responseParts = append(responseParts, genai.NewFunctionResponsePartFromURI(value.URI, value.MimeType))
			}
		default:
			return nil, nil, unsupportedContent("Google tool result cannot contain %q", block.BlockType())
		}
	}
	return map[string]any{key: text.String()}, responseParts, nil
}

func googleThinkingLevel(effort string) genai.ThinkingLevel {
	return genai.ThinkingLevel(strings.ToUpper(effort))
}

func googleResponse(result *genai.GenerateContentResponse) (*ModelCallResponse, error) {
	if result == nil || len(result.Candidates) == 0 || result.Candidates[0].Content == nil {
		return nil, fmt.Errorf("modelcall: Google GenAI returned no candidates")
	}

	candidate := result.Candidates[0]
	content := make(conversation.Content, 0, len(candidate.Content.Parts))
	for _, part := range candidate.Content.Parts {
		switch {
		case part.FunctionCall != nil:
			input, err := rawJSON(part.FunctionCall.Args)
			if err != nil {
				return nil, err
			}
			content = append(content, structuredToolUseBlock(
				part.FunctionCall.ID,
				part.FunctionCall.Name,
				input,
			))
		case part.InlineData != nil:
			content = append(content, conversation.ImageBlock{
				MimeType: part.InlineData.MIMEType,
				Data:     base64.StdEncoding.EncodeToString(part.InlineData.Data),
			})
		case part.FileData != nil:
			content = append(content, conversation.ImageBlock{
				MimeType: part.FileData.MIMEType, URI: part.FileData.FileURI,
			})
		case part.Thought:
			content = append(content, conversation.ReasoningBlock{
				Reasoning: part.Text,
				Signature: base64.StdEncoding.EncodeToString(part.ThoughtSignature),
			})
		case part.Text != "":
			content = append(content, conversation.TextBlock{Text: part.Text})
		}
	}

	usage := conversation.TokenUsage{}
	if result.UsageMetadata != nil {
		usage.Input = int64(result.UsageMetadata.PromptTokenCount)
		usage.Output = int64(result.UsageMetadata.CandidatesTokenCount)
		usage.CachedRead = int64(result.UsageMetadata.CachedContentTokenCount)
		usage.Reasoning = int64(result.UsageMetadata.ThoughtsTokenCount)
		usage.Total = int64(result.UsageMetadata.TotalTokenCount)
	}

	return &ModelCallResponse{
		ID: result.ResponseID, Model: result.ModelVersion, Content: content,
		Usage: usage, StopReason: googleStopReason(candidate.FinishReason),
	}, nil
}

func googleStopReason(reason genai.FinishReason) ModelCallStopReason {
	switch reason {
	case genai.FinishReasonMaxTokens:
		return ModelCallStopReasonMaxTokens
	case genai.FinishReasonSafety, genai.FinishReasonBlocklist,
		genai.FinishReasonProhibitedContent, genai.FinishReasonSPII:
		return ModelCallStopReasonContentFilter
	default:
		return ModelCallStopReasonEndTurn
	}
}

func emitGoogleChunk(handler ModelCallStreamHandler, chunk *genai.GenerateContentResponse) error {
	if chunk == nil {
		return nil
	}
	for _, candidate := range chunk.Candidates {
		if candidate.Content == nil {
			continue
		}
		for index, part := range candidate.Content.Parts {
			switch {
			case part.FunctionCall != nil:
				input, err := rawJSON(part.FunctionCall.Args)
				if err != nil {
					return err
				}
				if err := emit(handler, ModelCallStreamEvent{
					Type: ModelCallStreamEventToolUseStart, Index: index,
					ToolUseID: part.FunctionCall.ID, ToolName: part.FunctionCall.Name,
				}); err != nil {
					return err
				}
				if err := emit(handler, ModelCallStreamEvent{
					Type: ModelCallStreamEventToolUseDone, Index: index,
					ToolUseID: part.FunctionCall.ID, ToolName: part.FunctionCall.Name,
					ToolInput: input,
				}); err != nil {
					return err
				}
			case part.Thought && part.Text != "":
				if err := emit(handler, ModelCallStreamEvent{
					Type: ModelCallStreamEventReasoningDelta, Index: index, Delta: part.Text,
				}); err != nil {
					return err
				}
			case part.Text != "":
				if err := emit(handler, ModelCallStreamEvent{
					Type: ModelCallStreamEventTextDelta, Index: index, Delta: part.Text,
				}); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func mergeGoogleChunk(target, chunk *genai.GenerateContentResponse) {
	if chunk == nil {
		return
	}
	if chunk.ResponseID != "" {
		target.ResponseID = chunk.ResponseID
	}
	if chunk.ModelVersion != "" {
		target.ModelVersion = chunk.ModelVersion
	}
	if chunk.UsageMetadata != nil {
		target.UsageMetadata = chunk.UsageMetadata
	}
	if len(chunk.Candidates) == 0 {
		return
	}
	if len(target.Candidates) == 0 {
		target.Candidates = []*genai.Candidate{{Content: &genai.Content{Role: genai.RoleModel}}}
	}
	current := chunk.Candidates[0]
	if current.Content != nil {
		target.Candidates[0].Content.Parts = append(target.Candidates[0].Content.Parts, current.Content.Parts...)
	}
	if current.FinishReason != "" {
		target.Candidates[0].FinishReason = current.FinishReason
	}
}
