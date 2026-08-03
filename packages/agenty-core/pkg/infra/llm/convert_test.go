package llm

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	json "github.com/bytedance/sonic"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"google.golang.org/genai"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestNativeReasoningEffort(t *testing.T) {
	t.Parallel()

	model := testModel()
	tests := []struct {
		name    string
		effort  shared.ReasoningEffort
		want    string
		wantErr bool
	}{
		{name: "empty", effort: "", want: ""},
		{name: "off", effort: shared.ReasoningOff, want: ""},
		{name: "exact", effort: shared.ReasoningLow, want: "low"},
		{name: "mapped", effort: shared.ReasoningHigh, want: "HIGH"},
		{name: "unsupported", effort: shared.ReasoningMax, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := nativeReasoningEffort(model, tt.effort)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidRequest) {
					t.Fatalf("nativeReasoningEffort() error = %v, want ErrInvalidRequest", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("nativeReasoningEffort() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("nativeReasoningEffort() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProviderRequestConversions(t *testing.T) {
	t.Parallel()

	request := Request{
		SystemPrompt:    "Be concise.",
		Messages:        []conversation.Message{{Role: conversation.RoleUser, Content: conversation.Text("hello")}},
		Tools:           []ToolDefinition{testTool()},
		MaxOutputTokens: 128,
		ReasoningEffort: shared.ReasoningHigh,
	}

	t.Run("OpenAI Responses", func(t *testing.T) {
		t.Parallel()

		params, err := (&openAIResponsesCaller{model: modelWithReasoningNative("high")}).params(request)
		if err != nil {
			t.Fatalf("convert request: %v", err)
		}
		if params.Model != "test-model" || params.MaxOutputTokens.Value != 128 {
			t.Errorf("model/max tokens = %q/%d", params.Model, params.MaxOutputTokens.Value)
		}
		if params.Instructions.Value != "Be concise." || params.Reasoning.Effort != "high" {
			t.Errorf("instructions/reasoning = %q/%q", params.Instructions.Value, params.Reasoning.Effort)
		}
		if len(params.Input.OfInputItemList) != 1 || params.Input.OfInputItemList[0].OfMessage == nil {
			t.Fatalf("input = %#v, want one SDK message variant", params.Input.OfInputItemList)
		}
		message := params.Input.OfInputItemList[0].OfMessage
		if len(message.Content.OfInputItemContentList) != 1 ||
			message.Content.OfInputItemContentList[0].OfInputText == nil {
			t.Errorf("message content = %#v, want one SDK text variant", message.Content)
		}
		if len(params.Tools) != 1 || params.Tools[0].OfFunction == nil {
			t.Fatalf("tools = %#v, want one SDK function variant", params.Tools)
		}
		if params.Tools[0].OfFunction.Name != "lookup" || !params.Tools[0].OfFunction.Strict.Value {
			t.Errorf("function tool = %#v", params.Tools[0].OfFunction)
		}
	})

	t.Run("OpenAI Chat Completions", func(t *testing.T) {
		t.Parallel()

		params, err := (&openAIChatCaller{model: modelWithReasoningNative("high")}).params(request)
		if err != nil {
			t.Fatalf("convert request: %v", err)
		}
		if len(params.Messages) != 2 || params.Messages[0].OfSystem == nil || params.Messages[1].OfUser == nil {
			t.Fatalf("messages = %#v, want SDK system and user variants", params.Messages)
		}
		if params.MaxCompletionTokens.Value != 128 || params.ReasoningEffort != "high" {
			t.Errorf("max tokens/reasoning = %d/%q", params.MaxCompletionTokens.Value, params.ReasoningEffort)
		}
		if !params.StreamOptions.IncludeUsage.Value {
			t.Error("stream usage is disabled")
		}
		if len(params.Tools) != 1 || params.Tools[0].OfFunction == nil ||
			params.Tools[0].OfFunction.Function.Name != "lookup" {
			t.Errorf("tools = %#v, want one SDK function variant", params.Tools)
		}
	})

	t.Run("Anthropic Messages", func(t *testing.T) {
		t.Parallel()

		params, err := (&anthropicCaller{model: modelWithReasoningNative("high")}).params(request)
		if err != nil {
			t.Fatalf("convert request: %v", err)
		}
		if params.Model != "test-model" || params.MaxTokens != 128 {
			t.Errorf("model/max tokens = %q/%d", params.Model, params.MaxTokens)
		}
		if len(params.System) != 1 || params.System[0].Text != "Be concise." {
			t.Errorf("system = %#v", params.System)
		}
		if len(params.Messages) != 1 || len(params.Messages[0].Content) != 1 ||
			params.Messages[0].Content[0].OfText == nil {
			t.Fatalf("messages = %#v, want one SDK text block", params.Messages)
		}
		if len(params.Tools) != 1 || params.Tools[0].OfTool == nil ||
			params.Tools[0].OfTool.Name != "lookup" {
			t.Fatalf("tools = %#v, want one SDK tool variant", params.Tools)
		}
		if params.Thinking.OfAdaptive == nil || params.OutputConfig.Effort != "high" {
			t.Errorf("thinking/output config = %#v/%#v", params.Thinking, params.OutputConfig)
		}
	})

	t.Run("Google GenAI", func(t *testing.T) {
		t.Parallel()

		contents, config, err := (&googleCaller{model: modelWithReasoningNative("HIGH")}).params(request)
		if err != nil {
			t.Fatalf("convert request: %v", err)
		}
		if len(contents) != 1 || contents[0].Role != genai.RoleUser {
			t.Fatalf("contents = %#v, want one user message", contents)
		}
		if config.SystemInstruction == nil || config.ThinkingConfig == nil {
			t.Fatalf("config = %#v, want system and thinking config", config)
		}
		if config.ThinkingConfig.ThinkingLevel != genai.ThinkingLevelHigh {
			t.Errorf("thinking level = %q, want HIGH", config.ThinkingConfig.ThinkingLevel)
		}
		if len(config.Tools) != 1 || len(config.Tools[0].FunctionDeclarations) != 1 {
			t.Errorf("tools = %#v, want one declaration", config.Tools)
		}
	})
}

func modelWithReasoningNative(native string) catalog.Model {
	return catalog.Model{
		Slug: "test-model",
		ReasoningEffortMapping: map[string]shared.ReasoningEffort{
			native: shared.ReasoningHigh,
		},
	}
}

func TestToolDefinitionConversions(t *testing.T) {
	t.Parallel()

	tool := testTool()

	t.Run("OpenAI Responses", func(t *testing.T) {
		t.Parallel()

		converted, err := openAIResponsesToolDefinition(tool)
		if err != nil {
			t.Fatalf("convert tool: %v", err)
		}
		if converted.OfFunction == nil {
			t.Fatal("function tool variant is nil")
		}
		assertToolSchemaMap(t, converted.OfFunction.Parameters)
	})

	t.Run("OpenAI Chat Completions", func(t *testing.T) {
		t.Parallel()

		converted, err := openAIChatToolDefinition(tool)
		if err != nil {
			t.Fatalf("convert tool: %v", err)
		}
		if converted.OfFunction == nil {
			t.Fatal("function tool variant is nil")
		}
		assertToolSchemaMap(t, converted.OfFunction.Function.Parameters)
	})

	t.Run("Anthropic Messages", func(t *testing.T) {
		t.Parallel()

		converted, err := anthropicToolDefinition(tool)
		if err != nil {
			t.Fatalf("convert tool: %v", err)
		}
		if converted.OfTool == nil {
			t.Fatal("tool variant is nil")
		}
		schema := converted.OfTool.InputSchema
		if !slices.Equal(schema.Required, []string{"q"}) {
			t.Errorf("required = %#v, want [q]", schema.Required)
		}
		if _, ok := schema.Properties.(map[string]any); !ok {
			t.Errorf("properties = %#v, want object", schema.Properties)
		}
		if additional, ok := schema.ExtraFields["additionalProperties"].(bool); !ok || additional {
			t.Errorf("additionalProperties = %#v, want false", schema.ExtraFields["additionalProperties"])
		}
		for _, field := range []string{"type", "properties", "required"} {
			if _, ok := schema.ExtraFields[field]; ok {
				t.Errorf("ExtraFields unexpectedly contains %q", field)
			}
		}
	})

	t.Run("Google GenAI", func(t *testing.T) {
		t.Parallel()

		converted, err := googleToolDefinition(tool)
		if err != nil {
			t.Fatalf("convert tool: %v", err)
		}
		schema, ok := converted.ParametersJsonSchema.(map[string]any)
		if !ok {
			t.Fatalf("parameters schema = %#v, want object", converted.ParametersJsonSchema)
		}
		assertToolSchemaMap(t, schema)
	})
}

func TestToolDefinitionConversionsRejectNonObjectSchema(t *testing.T) {
	t.Parallel()

	tool := testTool()
	tool.InputSchema = JSONSchema{
		Type:  JSONSchemaTypeArray,
		Items: &JSONSchema{Type: JSONSchemaTypeString},
	}
	tests := []struct {
		name    string
		convert func() error
	}{
		{
			name: "OpenAI Responses",
			convert: func() error {
				_, err := openAIResponsesToolDefinition(tool)
				return err
			},
		},
		{
			name: "OpenAI Chat Completions",
			convert: func() error {
				_, err := openAIChatToolDefinition(tool)
				return err
			},
		},
		{
			name: "Anthropic Messages",
			convert: func() error {
				_, err := anthropicToolDefinition(tool)
				return err
			},
		},
		{
			name: "Google GenAI",
			convert: func() error {
				_, err := googleToolDefinition(tool)
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := tt.convert(); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("convert tool error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestProviderResponseConversions(t *testing.T) {
	t.Parallel()

	t.Run("OpenAI Responses", func(t *testing.T) {
		t.Parallel()

		var sdkResponse responses.Response
		mustUnmarshal(t, `{
			"id":"resp_1","model":"gpt-test","status":"completed",
			"output":[
				{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"brief"}],"encrypted_content":"opaque","status":"completed"},
				{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"done","annotations":[]}]},
				{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{\"q\":\"x\"}","status":"completed"}
			],
			"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15,"input_tokens_details":{"cached_tokens":2},"output_tokens_details":{"reasoning_tokens":1}}
		}`, &sdkResponse)

		response, err := openAIResponsesResponse(&sdkResponse)
		if err != nil {
			t.Fatalf("convert response: %v", err)
		}
		assertResponse(t, response, StopReasonToolUse, 3, 15)
		tool := response.Content[2].(conversation.ToolUseBlock)
		if string(tool.Input) != `{"q":"x"}` {
			t.Errorf("tool input = %s", tool.Input)
		}
	})

	t.Run("OpenAI Chat Completions", func(t *testing.T) {
		t.Parallel()

		var sdkResponse openai.ChatCompletion
		mustUnmarshal(t, `{
			"id":"chat_1","model":"gpt-test",
			"choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"x\"}"}}]}}],
			"usage":{"prompt_tokens":8,"completion_tokens":4,"total_tokens":12,"prompt_tokens_details":{"cached_tokens":1},"completion_tokens_details":{"reasoning_tokens":2}}
		}`, &sdkResponse)

		response, err := openAIChatResponse(&sdkResponse)
		if err != nil {
			t.Fatalf("convert response: %v", err)
		}
		assertResponse(t, response, StopReasonToolUse, 1, 12)
	})

	t.Run("Anthropic Messages", func(t *testing.T) {
		t.Parallel()

		var sdkResponse anthropic.Message
		mustUnmarshal(t, `{
			"id":"msg_1","model":"claude-test","role":"assistant","type":"message","stop_reason":"tool_use",
			"content":[
				{"type":"thinking","thinking":"brief","signature":"sig"},
				{"type":"text","text":"done"},
				{"type":"tool_use","id":"call_1","name":"lookup","input":{"q":"x"}}
			],
			"usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":2,"cache_read_input_tokens":3}
		}`, &sdkResponse)

		response, err := anthropicResponse(&sdkResponse)
		if err != nil {
			t.Fatalf("convert response: %v", err)
		}
		assertResponse(t, response, StopReasonToolUse, 3, 15)
	})

	t.Run("Google GenAI", func(t *testing.T) {
		t.Parallel()

		result := &genai.GenerateContentResponse{
			ResponseID: "google_1", ModelVersion: "gemini-test",
			Candidates: []*genai.Candidate{{
				FinishReason: genai.FinishReasonStop,
				Content: genai.NewContentFromParts([]*genai.Part{
					{Text: "thinking", Thought: true, ThoughtSignature: []byte("sig")},
					genai.NewPartFromText("done"),
					genai.NewPartFromFunctionCall("lookup", map[string]any{"q": "x"}),
				}, genai.RoleModel),
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount: 9, CandidatesTokenCount: 3, ThoughtsTokenCount: 2, TotalTokenCount: 14,
			},
		}
		result.Candidates[0].Content.Parts[2].FunctionCall.ID = "call_1"

		response, err := googleResponse(result)
		if err != nil {
			t.Fatalf("convert response: %v", err)
		}
		assertResponse(t, response, StopReasonEndTurn, 3, 14)
	})
}

func TestSDKMessageConversionsUseTypedVariants(t *testing.T) {
	t.Parallel()

	t.Run("OpenAI Responses", func(t *testing.T) {
		t.Parallel()

		items, err := openAIResponsesMessage(conversation.Message{
			Role: conversation.RoleAssistant,
			Content: conversation.Content{
				conversation.ReasoningBlock{Extra: shared.RawJSON(`{
					"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"opaque"
				}`)},
				conversation.ToolUseBlock{ID: "call_1", Name: "lookup", Input: shared.RawJSON(`{"q":"x"}`)},
			},
		})
		if err != nil {
			t.Fatalf("convert message: %v", err)
		}
		if len(items) != 2 || items[0].OfReasoning == nil || items[1].OfFunctionCall == nil {
			t.Fatalf("items = %#v, want reasoning and function-call SDK variants", items)
		}
		if items[1].OfFunctionCall.Arguments != `{"q":"x"}` {
			t.Errorf("function arguments = %q", items[1].OfFunctionCall.Arguments)
		}
	})

	t.Run("OpenAI Chat Completions", func(t *testing.T) {
		t.Parallel()

		messages, err := openAIChatMessages(conversation.Message{
			Role: conversation.RoleAssistant,
			Content: conversation.Content{
				conversation.TextBlock{Text: "working"},
				conversation.ToolUseBlock{ID: "call_1", Name: "lookup", Input: shared.RawJSON(`{"q":"x"}`)},
			},
		})
		if err != nil {
			t.Fatalf("convert message: %v", err)
		}
		if len(messages) != 1 || messages[0].OfAssistant == nil {
			t.Fatalf("messages = %#v, want one assistant SDK variant", messages)
		}
		calls := messages[0].OfAssistant.ToolCalls
		if len(calls) != 1 || calls[0].OfFunction == nil || calls[0].OfFunction.Function.Name != "lookup" {
			t.Errorf("tool calls = %#v, want one function SDK variant", calls)
		}
	})

	t.Run("Anthropic Messages", func(t *testing.T) {
		t.Parallel()

		message, err := anthropicMessage(conversation.Message{
			Role: conversation.RoleAssistant,
			Content: conversation.Content{
				conversation.ReasoningBlock{Reasoning: "brief", Signature: "sig"},
				conversation.ToolUseBlock{ID: "call_1", Name: "lookup", Input: shared.RawJSON(`{"q":"x"}`)},
			},
		})
		if err != nil {
			t.Fatalf("convert message: %v", err)
		}
		if message.Role != anthropic.MessageParamRoleAssistant || len(message.Content) != 2 {
			t.Fatalf("message = %#v, want assistant with two SDK blocks", message)
		}
		if message.Content[0].OfThinking == nil || message.Content[1].OfToolUse == nil {
			t.Errorf("content = %#v, want thinking and tool-use SDK variants", message.Content)
		}
	})

	t.Run("Google GenAI", func(t *testing.T) {
		t.Parallel()

		content, err := googleMessage(conversation.Message{
			Role: conversation.RoleAssistant,
			Content: conversation.Content{
				conversation.ToolUseBlock{ID: "call_1", Name: "lookup", Input: shared.RawJSON(`{"q":"x"}`)},
			},
		}, nil)
		if err != nil {
			t.Fatalf("convert message: %v", err)
		}
		if len(content.Parts) != 1 || content.Parts[0].FunctionCall == nil ||
			content.Parts[0].FunctionCall.ID != "call_1" {
			t.Errorf("content = %#v, want one function-call SDK part", content)
		}
	})
}

func TestStreamEventConversions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(StreamHandler) error
		want StreamEvent
	}{
		{
			name: "OpenAI text delta",
			run: func(handler StreamHandler) error {
				return emitOpenAIResponsesEvent(handler, openAIResponsesStreamEvent{
					Type: "response.output_text.delta", OutputIndex: 2, Delta: "hello",
				})
			},
			want: StreamEvent{Type: StreamEventTextDelta, Index: 2, Delta: "hello"},
		},
		{
			name: "Anthropic tool JSON delta",
			run: func(handler StreamHandler) error {
				event := anthropicStreamEvent{Type: "content_block_delta", Index: 1}
				event.Delta.Type = "input_json_delta"
				event.Delta.PartialJSON = `{"q"`
				return emitAnthropicEvent(handler, event)
			},
			want: StreamEvent{Type: StreamEventToolInputDelta, Index: 1, Delta: `{"q"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got StreamEvent
			if err := tt.run(func(event StreamEvent) error {
				got = event
				return nil
			}); err != nil {
				t.Fatalf("convert stream event: %v", err)
			}
			if got.Type != tt.want.Type || got.Index != tt.want.Index || got.Delta != tt.want.Delta {
				t.Errorf("event = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNewCallerValidation(t *testing.T) {
	t.Parallel()

	model := testModel()
	tests := []struct {
		name     string
		provider catalog.Provider
		want     error
	}{
		{
			name:     "missing API key",
			provider: catalog.Provider{Slug: "openai", Type: catalog.APIOpenAI},
			want:     ErrInvalidRequest,
		},
		{
			name:     "unsupported API",
			provider: catalog.Provider{Slug: "deepseek", Type: catalog.APIDeepSeek, APIKey: "test"},
			want:     ErrUnsupportedAPI,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewCaller(context.Background(), tt.provider, model)
			if !errors.Is(err, tt.want) {
				t.Fatalf("NewCaller() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func testModel() catalog.Model {
	return catalog.Model{
		Slug: "test-model",
		ReasoningEffortMapping: map[string]shared.ReasoningEffort{
			"low": shared.ReasoningLow, "HIGH": shared.ReasoningHigh,
		},
	}
}

func testTool() ToolDefinition {
	return ToolDefinition{
		Name: "lookup", Description: "Look up a value",
		InputSchema: JSONSchema{
			Type: JSONSchemaTypeObject,
			Properties: map[string]JSONSchema{
				"q": {Type: JSONSchemaTypeString},
			},
			Required:             []string{"q"},
			AdditionalProperties: AllowAdditionalProperties(false),
		},
		Strict: true,
	}
}

func assertToolSchemaMap(t *testing.T, schema map[string]any) {
	t.Helper()
	if schema["type"] != "object" {
		t.Errorf("type = %#v, want object", schema["type"])
	}
	if additional, ok := schema["additionalProperties"].(bool); !ok || additional {
		t.Errorf("additionalProperties = %#v, want false", schema["additionalProperties"])
	}
	required, ok := schema["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "q" {
		t.Errorf("required = %#v, want [q]", schema["required"])
	}
}

func mustUnmarshal(t *testing.T, data string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(data), target); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
}

func assertResponse(t *testing.T, response *Response, reason StopReason, contentLength int, total int64) {
	t.Helper()
	if response.StopReason != reason {
		t.Errorf("stop reason = %q, want %q", response.StopReason, reason)
	}
	if len(response.Content) != contentLength {
		t.Errorf("content length = %d, want %d", len(response.Content), contentLength)
	}
	if response.Usage.Total != total {
		t.Errorf("total usage = %d, want %d", response.Usage.Total, total)
	}
}
