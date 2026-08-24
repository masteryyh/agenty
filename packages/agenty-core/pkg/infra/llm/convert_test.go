package llm

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	json "github.com/bytedance/sonic"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"google.golang.org/genai"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestModelReasoningEffort(t *testing.T) {
	t.Parallel()

	model := testModel()
	tests := []struct {
		name   string
		effort shared.ReasoningEffort
		want   string
	}{
		{name: "empty", effort: "", want: ""},
		{name: "off", effort: shared.ReasoningOff, want: ""},
		{name: "exact", effort: shared.ReasoningLow, want: "low"},
		{name: "high", effort: shared.ReasoningHigh, want: "high"},
		{name: "max", effort: shared.ReasoningMax, want: "max"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := modelReasoningEffort(model, tt.effort)
			if got != tt.want {
				t.Fatalf("modelReasoningEffort() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestModelReasoningEffortSendsUnsupportedLevelToUpstream(t *testing.T) {
	model := catalog.Model{
		Code:             "gpt-5-mini",
		ReasoningEfforts: []shared.ReasoningEffort{shared.ReasoningLow, shared.ReasoningHigh},
	}
	got := modelReasoningEffort(model, shared.ReasoningMax)
	if got != "max" {
		t.Fatalf("modelReasoningEffort() = %q, want max", got)
	}
}

func TestModelReasoningEffortIgnoresNonReasoningModel(t *testing.T) {
	model := catalog.Model{Code: "gpt-4o", ReasoningEfforts: []shared.ReasoningEffort{}}
	got := modelReasoningEffort(model, shared.ReasoningHigh)
	if got != "" {
		t.Fatalf("modelReasoningEffort() = %q, want empty", got)
	}
}

func TestGoogleThinkingLevelPassesStandardEffortThrough(t *testing.T) {
	if got := googleThinkingLevel("max"); got != genai.ThinkingLevel("MAX") {
		t.Fatalf("googleThinkingLevel(max) = %q, want MAX", got)
	}
}

func TestProviderRequestConversions(t *testing.T) {
	t.Parallel()

	request := modelRequest{
		SystemPrompt:    "Be concise.",
		Messages:        []conversation.Message{{Role: conversation.RoleUser, Content: conversation.Text("hello")}},
		Tools:           []modelToolDefinition{testTool()},
		MaxOutputTokens: 128,
		ReasoningEffort: shared.ReasoningHigh,
	}

	t.Run("OpenAI Responses", func(t *testing.T) {
		t.Parallel()

		params, err := (&openAIResponsesCaller{model: reasoningModel()}).params(request)
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

		params, err := (&openAIChatCaller{model: reasoningModel()}).params(request)
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

		params, err := (&anthropicCaller{model: reasoningModel()}).params(request)
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

		contents, config, err := (&googleCaller{model: reasoningModel()}).params(request)
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

func reasoningModel() catalog.Model {
	return catalog.Model{
		Code:             "test-model",
		ReasoningEfforts: []shared.ReasoningEffort{shared.ReasoningHigh},
	}
}

func TestToolDefinitionConversions(t *testing.T) {
	t.Parallel()

	tool := testTool()

	t.Run("OpenAI Responses", func(t *testing.T) {
		t.Parallel()

		converted, err := openAIResponsesToolDefinition(tool, false)
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

func TestShellToolDefinitionConversions(t *testing.T) {
	t.Parallel()

	tool := testShellTool()

	openAIResponses, err := openAIResponsesToolDefinition(tool, true)
	if err != nil {
		t.Fatal(err)
	}
	if openAIResponses.OfShell == nil || openAIResponses.OfShell.Environment.OfLocal == nil {
		t.Fatalf("OpenAI Responses shell = %#v, want native local shell", openAIResponses)
	}
	compatibleResponses, err := openAIResponsesToolDefinition(tool, false)
	if err != nil {
		t.Fatal(err)
	}
	if compatibleResponses.OfFunction == nil || compatibleResponses.OfFunction.Name != "shell" {
		t.Fatalf("compatible Responses shell = %#v, want function", compatibleResponses)
	}
	if compatibleResponses.OfFunction.Strict.Value {
		t.Error("compatible Responses shell strict = true, want false")
	}
	assertShellToolSchemaMap(t, compatibleResponses.OfFunction.Parameters)

	openAIChat, err := openAIChatToolDefinition(tool)
	if err != nil {
		t.Fatal(err)
	}
	if openAIChat.OfFunction == nil || openAIChat.OfFunction.Function.Name != "shell" {
		t.Fatalf("OpenAI Chat shell = %#v, want function", openAIChat)
	}
	if openAIChat.OfFunction.Function.Strict.Value {
		t.Error("OpenAI Chat shell strict = true, want false")
	}

	anthropicTool, err := anthropicToolDefinition(tool)
	if err != nil {
		t.Fatal(err)
	}
	if anthropicTool.OfTool == nil || anthropicTool.OfTool.Name != "shell" {
		t.Fatalf("Anthropic shell = %#v, want custom tool", anthropicTool)
	}

	googleTool, err := googleToolDefinition(tool)
	if err != nil {
		t.Fatal(err)
	}
	if googleTool.Name != "shell" {
		t.Fatalf("Google shell name = %q", googleTool.Name)
	}
}

func TestApplyPatchToolRegistrations(t *testing.T) {
	t.Parallel()

	definitions := []modelToolDefinition{
		testNamedTool("read_file"),
		testApplyPatchTool(),
	}

	filesystem, err := openAIResponsesTools(definitions, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(filesystem) != 1 || filesystem[0].OfFunction == nil || filesystem[0].OfFunction.Name != "read_file" {
		t.Fatalf("non-free-form Responses tools = %#v, want read_file only", filesystem)
	}

	freeForm, err := openAIResponsesTools(definitions, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(freeForm) != 2 || freeForm[0].OfFunction == nil || freeForm[0].OfFunction.Name != "read_file" ||
		freeForm[1].OfCustom == nil || freeForm[1].OfCustom.Name != "apply_patch" {
		t.Fatalf("free-form Responses tools = %#v, want read_file and custom apply_patch", freeForm)
	}

	chat, err := openAIChatTools(definitions)
	if err != nil {
		t.Fatal(err)
	}
	if names := openAIChatToolNames(chat); !slices.Equal(names, []string{"read_file"}) {
		t.Errorf("OpenAI Chat tools = %q", names)
	}

	anthropicDefinitions, err := anthropicTools(definitions)
	if err != nil {
		t.Fatal(err)
	}
	if len(anthropicDefinitions) != 1 {
		t.Errorf("Anthropic tools = %d, want read_file only", len(anthropicDefinitions))
	}

	googleDefinitions, err := googleTools(definitions)
	if err != nil {
		t.Fatal(err)
	}
	if len(googleDefinitions) != 1 {
		t.Errorf("Google tools = %#v, want read_file only", googleDefinitions)
	}
}

func openAIChatToolNames(tools []openai.ChatCompletionToolUnionParam) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool.OfFunction != nil {
			names = append(names, tool.OfFunction.Function.Name)
		}
	}
	return names
}

func TestProviderToolRegistrationsRejectUnknownToolType(t *testing.T) {
	t.Parallel()

	tool := testTool()
	tool.Type = "unknown"
	checks := []func() error{
		func() error {
			_, err := openAIResponsesTools([]modelToolDefinition{tool}, false)
			return err
		},
		func() error {
			_, err := openAIChatTools([]modelToolDefinition{tool})
			return err
		},
		func() error {
			_, err := anthropicTools([]modelToolDefinition{tool})
			return err
		},
		func() error {
			_, err := googleTools([]modelToolDefinition{tool})
			return err
		},
	}
	for index, check := range checks {
		if err := check(); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("provider registration %d error = %v, want ErrInvalidRequest", index, err)
		}
	}
}

func TestToolDefinitionConversionsRejectNonObjectSchema(t *testing.T) {
	t.Parallel()

	tool := testTool()
	tool.InputSchema = toolJSONSchema{
		Type:  toolJSONSchemaTypeArray,
		Items: &toolJSONSchema{Type: toolJSONSchemaTypeString},
	}
	tests := []struct {
		name    string
		convert func() error
	}{
		{
			name: "OpenAI Responses",
			convert: func() error {
				_, err := openAIResponsesToolDefinition(tool, false)
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
		assertResponse(t, response, modelStopReasonToolUse, 3, 15)
		tool := response.Content[2].(conversation.ToolUseBlock)
		if string(tool.Input) != `{"q":"x"}` {
			t.Errorf("tool input = %s", tool.Input)
		}
	})

	t.Run("OpenAI Responses shell call", func(t *testing.T) {
		t.Parallel()

		var sdkResponse responses.Response
		mustUnmarshal(t, `{
			"id":"resp_shell","model":"gpt-test","status":"completed",
			"output":[{
				"type":"shell_call","id":"sh_1","call_id":"call_1","status":"completed",
				"environment":{"type":"local"},
				"action":{"commands":["pwd","false"],"timeout_ms":1000,"max_output_length":4096}
			}],
			"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
		}`, &sdkResponse)

		response, err := openAIResponsesResponse(&sdkResponse)
		if err != nil {
			t.Fatal(err)
		}
		assertResponse(t, response, modelStopReasonToolUse, 1, 7)
		call, ok := response.Content[0].(conversation.ShellCallBlock)
		if !ok || call.CallID != "call_1" || !slices.Equal(call.Commands, []string{"pwd", "false"}) {
			t.Fatalf("shell call = %#v", response.Content[0])
		}
	})

	t.Run("OpenAI Responses native apply patch call", func(t *testing.T) {
		t.Parallel()

		var sdkResponse responses.Response
		mustUnmarshal(t, `{
			"id":"resp_patch","model":"gpt-test","status":"completed",
			"output":[{
				"type":"apply_patch_call","id":"apc_1","call_id":"call_1","status":"completed",
				"operation":{"type":"update_file","path":"main.go","diff":"@@\n-old\n+new"}
			}],
			"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
		}`, &sdkResponse)

		response, err := openAIResponsesResponse(&sdkResponse)
		if err != nil {
			t.Fatal(err)
		}
		assertResponse(t, response, modelStopReasonToolUse, 1, 7)
		call, ok := response.Content[0].(conversation.ApplyPatchCallBlock)
		if !ok || call.Source != conversation.ApplyPatchSourceNative || call.Operation == nil ||
			call.Operation.Type != conversation.ApplyPatchUpdateFile || call.Operation.Path != "main.go" {
			t.Fatalf("apply patch call = %#v", response.Content[0])
		}
	})

	t.Run("OpenAI Responses custom apply patch call", func(t *testing.T) {
		t.Parallel()

		const patch = "*** Begin Patch\n*** Delete File: old.txt\n*** End Patch"
		var sdkResponse responses.Response
		mustUnmarshal(t, `{
			"id":"resp_patch","model":"gpt-test","status":"completed",
			"output":[{
				"type":"custom_tool_call","id":"ctc_1","call_id":"call_1","name":"apply_patch",
				"input":"*** Begin Patch\n*** Delete File: old.txt\n*** End Patch","status":"completed"
			}],
			"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
		}`, &sdkResponse)

		response, err := openAIResponsesResponse(&sdkResponse)
		if err != nil {
			t.Fatal(err)
		}
		call, ok := response.Content[0].(conversation.ApplyPatchCallBlock)
		if !ok || call.Source != conversation.ApplyPatchSourceCustom || call.Patch != patch {
			t.Fatalf("custom apply patch call = %#v", response.Content[0])
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
		assertResponse(t, response, modelStopReasonToolUse, 1, 12)
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
		assertResponse(t, response, modelStopReasonToolUse, 3, 15)
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
		assertResponse(t, response, modelStopReasonEndTurn, 3, 14)
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
		}, false)
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

func TestShellMessageConversionsAcrossProviders(t *testing.T) {
	t.Parallel()

	exitCode := int64(0)
	call := conversation.ShellCallBlock{
		ID: "sh_1", CallID: "call_1", Commands: []string{"printf hi"},
		TimeoutMs: 1000, MaxOutputLength: 4096,
	}
	result := conversation.Message{
		Role: conversation.RoleUser,
		Content: conversation.Content{conversation.ToolResultBlock{
			ToolUseID: "call_1",
			Content: conversation.Content{conversation.ShellCallOutputBlock{
				CallID: "call_1", MaxOutputLength: 4096, OpenAINative: boolPointer(true),
				Output: []conversation.ShellCommandOutput{{
					Stdout: "hi", Outcome: conversation.ShellOutcome{Type: "exit", ExitCode: &exitCode},
				}},
			}},
		}},
	}

	responsesCall, err := openAIResponsesMessage(conversation.Message{
		Role: conversation.RoleAssistant, Content: conversation.Content{call},
	}, true)
	if err != nil || len(responsesCall) != 1 || responsesCall[0].OfShellCall == nil {
		t.Fatalf("Responses shell call = %#v, err = %v", responsesCall, err)
	}
	if responsesCall[0].OfShellCall.Action.Commands[0] != "printf hi" {
		t.Errorf("Responses commands = %#v", responsesCall[0].OfShellCall.Action.Commands)
	}

	responsesResult, err := openAIResponsesMessage(result, true)
	if err != nil || len(responsesResult) != 1 || responsesResult[0].OfShellCallOutput == nil {
		t.Fatalf("Responses shell output = %#v, err = %v", responsesResult, err)
	}
	if len(responsesResult[0].OfShellCallOutput.Output) != 1 {
		t.Errorf("Responses output = %#v", responsesResult[0].OfShellCallOutput.Output)
	}

	compatibleCall, err := openAIResponsesMessage(conversation.Message{
		Role: conversation.RoleAssistant, Content: conversation.Content{call},
	}, false)
	if err != nil || len(compatibleCall) != 1 || compatibleCall[0].OfFunctionCall == nil {
		t.Fatalf("compatible Responses shell call = %#v, err = %v", compatibleCall, err)
	}
	if compatibleCall[0].OfFunctionCall.Name != "shell" ||
		compatibleCall[0].OfFunctionCall.Arguments != `{"commands":["printf hi"],"timeout_ms":1000,"max_output_length":4096}` {
		t.Errorf("compatible Responses shell call = %#v", compatibleCall[0].OfFunctionCall)
	}
	compatibleResult, err := openAIResponsesMessage(result, false)
	if err != nil || len(compatibleResult) != 1 || compatibleResult[0].OfFunctionCallOutput == nil {
		t.Fatalf("compatible Responses shell result = %#v, err = %v", compatibleResult, err)
	}
	if compatibleResult[0].OfFunctionCallOutput.Output.OfString.Value != `{"type":"shell_call_output","call_id":"call_1","max_output_length":4096,"output":[{"stdout":"hi","stderr":"","outcome":{"type":"exit","exit_code":0}}]}` {
		t.Errorf("compatible Responses shell result = %#v", compatibleResult[0].OfFunctionCallOutput.Output)
	}

	chatCall, err := openAIChatMessages(conversation.Message{
		Role: conversation.RoleAssistant, Content: conversation.Content{call},
	})
	if err != nil || len(chatCall) != 1 || len(chatCall[0].OfAssistant.ToolCalls) != 1 {
		t.Fatalf("Chat shell call = %#v, err = %v", chatCall, err)
	}
	chatResult, err := openAIChatMessages(result)
	if err != nil || len(chatResult) != 1 || chatResult[0].OfTool == nil {
		t.Fatalf("Chat shell output = %#v, err = %v", chatResult, err)
	}
	if chatResult[0].OfTool.Content.OfString.Value != `{"type":"shell_call_output","call_id":"call_1","max_output_length":4096,"output":[{"stdout":"hi","stderr":"","outcome":{"type":"exit","exit_code":0}}]}` {
		t.Errorf("Chat shell output JSON = %q", chatResult[0].OfTool.Content.OfString.Value)
	}

	anthropicCall, err := anthropicMessage(conversation.Message{
		Role: conversation.RoleAssistant, Content: conversation.Content{call},
	})
	if err != nil || len(anthropicCall.Content) != 1 || anthropicCall.Content[0].OfToolUse == nil {
		t.Fatalf("Anthropic shell call = %#v, err = %v", anthropicCall, err)
	}
	anthropicResult, err := anthropicMessage(result)
	if err != nil || len(anthropicResult.Content) != 1 || anthropicResult.Content[0].OfToolResult == nil {
		t.Fatalf("Anthropic shell output = %#v, err = %v", anthropicResult, err)
	}
	if len(anthropicResult.Content[0].OfToolResult.Content) != 1 ||
		anthropicResult.Content[0].OfToolResult.Content[0].OfText == nil ||
		anthropicResult.Content[0].OfToolResult.Content[0].OfText.Text != `{"type":"shell_call_output","call_id":"call_1","max_output_length":4096,"output":[{"stdout":"hi","stderr":"","outcome":{"type":"exit","exit_code":0}}]}` {
		t.Errorf("Anthropic shell output JSON = %#v", anthropicResult.Content[0].OfToolResult.Content)
	}

	googleCall, err := googleMessage(conversation.Message{
		Role: conversation.RoleAssistant, Content: conversation.Content{call},
	}, nil)
	if err != nil || len(googleCall.Parts) != 1 || googleCall.Parts[0].FunctionCall == nil {
		t.Fatalf("Google shell call = %#v, err = %v", googleCall, err)
	}
	googleResult, err := googleMessage(result, map[string]string{"call_1": "shell"})
	if err != nil || len(googleResult.Parts) != 1 || googleResult.Parts[0].FunctionResponse == nil {
		t.Fatalf("Google shell output = %#v, err = %v", googleResult, err)
	}
	if googleResult.Parts[0].FunctionResponse.Response["type"] != "shell_call_output" ||
		googleResult.Parts[0].FunctionResponse.Response["call_id"] != "call_1" {
		t.Errorf("Google shell output response = %#v", googleResult.Parts[0].FunctionResponse.Response)
	}
	if _, ok := googleResult.Parts[0].FunctionResponse.Response["exitCode"]; ok {
		t.Errorf("Google shell output response contains camelCase exitCode: %#v", googleResult.Parts[0].FunctionResponse.Response)
	}
}

func TestApplyPatchMessageConversionsAcrossProviders(t *testing.T) {
	t.Parallel()

	operation := conversation.ApplyPatchOperation{
		Type: conversation.ApplyPatchUpdateFile,
		Path: "notes.txt",
		Diff: "@@\n-old\n+new",
	}
	call := conversation.ApplyPatchCallBlock{
		ID: "apc_1", CallID: "call_1", Source: conversation.ApplyPatchSourceNative,
		Operation: &operation,
	}
	assistant := conversation.Message{
		Role: conversation.RoleAssistant, Content: conversation.Content{call},
	}
	result := conversation.Message{
		Role: conversation.RoleUser,
		Content: conversation.Content{conversation.ToolResultBlock{
			ToolUseID: "call_1", Content: conversation.Text(`{"operations":[{"type":"update_file","path":"notes.txt"}]}`),
		}},
	}

	nativeItems, err := openAIResponsesMessages([]conversation.Message{assistant, result}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(nativeItems) != 2 || nativeItems[0].OfApplyPatchCall == nil ||
		nativeItems[1].OfApplyPatchCallOutput == nil || nativeItems[1].OfApplyPatchCallOutput.Status != "completed" {
		t.Fatalf("native Responses history = %#v", nativeItems)
	}

	freeFormItems, err := openAIResponsesMessages([]conversation.Message{assistant, result}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(freeFormItems) != 2 || freeFormItems[0].OfCustomToolCall == nil ||
		freeFormItems[1].OfCustomToolCallOutput == nil {
		t.Fatalf("free-form Responses history = %#v", freeFormItems)
	}

	compatibleItems, err := openAIResponsesMessages([]conversation.Message{assistant, result}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(compatibleItems) != 2 || compatibleItems[0].OfCustomToolCall == nil ||
		compatibleItems[1].OfCustomToolCallOutput == nil {
		t.Fatalf("compatible Responses history = %#v", compatibleItems)
	}
	wantPatch := "*** Begin Patch\n*** Update File: notes.txt\n@@\n-old\n+new\n*** End Patch"
	if compatibleItems[0].OfCustomToolCall.Input != wantPatch {
		t.Errorf("compatible patch = %q, want %q", compatibleItems[0].OfCustomToolCall.Input, wantPatch)
	}

	customCall := conversation.ApplyPatchCallBlock{
		ID: "ctc_1", CallID: "call_2", Source: conversation.ApplyPatchSourceCustom,
		Patch: "*** Begin Patch\n*** Delete File: old.txt\n*** End Patch",
	}
	customItems, err := openAIResponsesMessages([]conversation.Message{
		{Role: conversation.RoleAssistant, Content: conversation.Content{customCall}},
		{Role: conversation.RoleUser, Content: conversation.Content{conversation.ToolResultBlock{
			ToolUseID: "call_2", Content: conversation.Text(`{"operations":[{"type":"delete_file","path":"old.txt"}]}`),
		}}},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(customItems) != 2 || customItems[0].OfCustomToolCall == nil ||
		customItems[1].OfCustomToolCallOutput == nil {
		t.Fatalf("custom Responses history = %#v", customItems)
	}

	chatMessages, err := openAIChatMessages(assistant)
	if err != nil || len(chatMessages) != 1 || len(chatMessages[0].OfAssistant.ToolCalls) != 1 ||
		chatMessages[0].OfAssistant.ToolCalls[0].OfFunction.Function.Name != "apply_patch" {
		t.Fatalf("Chat apply patch history = %#v, err = %v", chatMessages, err)
	}
	anthropicMessage, err := anthropicMessage(assistant)
	if err != nil || len(anthropicMessage.Content) != 1 ||
		anthropicMessage.Content[0].OfToolUse == nil || anthropicMessage.Content[0].OfToolUse.Name != "apply_patch" {
		t.Fatalf("Anthropic apply patch history = %#v, err = %v", anthropicMessage, err)
	}
	googleMessage, err := googleMessage(assistant, nil)
	if err != nil || len(googleMessage.Parts) != 1 ||
		googleMessage.Parts[0].FunctionCall == nil || googleMessage.Parts[0].FunctionCall.Name != "apply_patch" {
		t.Fatalf("Google apply patch history = %#v, err = %v", googleMessage, err)
	}
	if name := googleToolNames([]conversation.Message{assistant})["call_1"]; name != "apply_patch" {
		t.Errorf("Google apply patch tool name = %q", name)
	}
}

func TestOpenAIResponsesShellOutputUsesPersistedSource(t *testing.T) {
	t.Parallel()

	exitCode := int64(0)
	for _, test := range []struct {
		name       string
		native     *bool
		wantNative bool
	}{
		{name: "native", native: boolPointer(true), wantNative: true},
		{name: "function", native: boolPointer(false), wantNative: false},
		{name: "legacy native", native: nil, wantNative: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := conversation.Message{
				Role: conversation.RoleUser,
				Content: conversation.Content{conversation.ToolResultBlock{
					ToolUseID: "call_1",
					Content: conversation.Content{conversation.ShellCallOutputBlock{
						CallID: "call_1", MaxOutputLength: 4096, OpenAINative: test.native,
						Output: []conversation.ShellCommandOutput{{
							Stdout: "hi", Outcome: conversation.ShellOutcome{Type: "exit", ExitCode: &exitCode},
						}},
					}},
				}},
			}
			if test.name == "legacy native" {
				items, err := openAIResponsesMessages([]conversation.Message{
					{Role: conversation.RoleAssistant, Content: conversation.Content{
						conversation.ShellCallBlock{CallID: "call_1", Commands: []string{"printf hi"}},
					}},
					result,
				}, true)
				if err != nil || len(items) != 2 || items[1].OfShellCallOutput == nil {
					t.Fatalf("legacy native result = %#v, err = %v", items, err)
				}
				return
			}

			items, err := openAIResponsesMessage(result, true)
			if err != nil || len(items) != 1 {
				t.Fatalf("result = %#v, err = %v", items, err)
			}
			if test.wantNative && items[0].OfShellCallOutput == nil {
				t.Fatalf("result = %#v, want shell_call_output", items[0])
			}
			if !test.wantNative && items[0].OfFunctionCallOutput == nil {
				t.Fatalf("result = %#v, want function_call_output", items[0])
			}
		})
	}
}

func TestStreamEventConversions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(modelStreamHandler) error
		want modelStreamEvent
	}{
		{
			name: "OpenAI text delta",
			run: func(handler modelStreamHandler) error {
				return emitOpenAIResponsesEvent(handler, openAIResponsesStreamEvent{
					Type: "response.output_text.delta", OutputIndex: 2, Delta: "hello",
				})
			},
			want: modelStreamEvent{Type: modelStreamEventTextDelta, Index: 2, Delta: "hello"},
		},
		{
			name: "Anthropic tool JSON delta",
			run: func(handler modelStreamHandler) error {
				event := anthropicStreamEvent{Type: "content_block_delta", Index: 1}
				event.Delta.Type = "input_json_delta"
				event.Delta.PartialJSON = `{"q"`
				return emitAnthropicEvent(handler, event)
			},
			want: modelStreamEvent{Type: modelStreamEventToolInputDelta, Index: 1, Delta: `{"q"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got modelStreamEvent
			if err := tt.run(func(event modelStreamEvent) error {
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

func TestOpenAIResponsesCustomApplyPatchStreamUsesCompletedFreeformInput(t *testing.T) {
	t.Parallel()

	called := false
	err := emitOpenAIResponsesEvent(func(modelStreamEvent) error {
		called = true
		return nil
	}, openAIResponsesStreamEvent{
		Type: "response.custom_tool_call_input.delta", OutputIndex: 1,
		Delta: "*** Begin Patch",
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("custom freeform delta was emitted as a JSON tool input delta")
	}

	done := openAIResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 1}
	done.Item.Type = "custom_tool_call"
	done.Item.CallID = "call_1"
	done.Item.Name = "apply_patch"
	done.Item.Input = "*** Begin Patch\n*** Delete File: old.txt\n*** End Patch"
	var got modelStreamEvent
	if err := emitOpenAIResponsesEvent(func(event modelStreamEvent) error {
		got = event
		return nil
	}, done); err != nil {
		t.Fatal(err)
	}
	if got.Type != modelStreamEventToolUseDone || got.ToolUseID != "call_1" ||
		!strings.Contains(string(got.ToolInput), `"patch":"*** Begin Patch`) {
		t.Errorf("completed custom apply patch event = %#v", got)
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
			provider: catalog.Provider{Code: "openai", Type: catalog.APIOpenAI},
			want:     ErrInvalidRequest,
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

func TestNewCallerConfiguresNativeOpenAIResponsesToolsByProviderIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		provider     catalog.Provider
		wantNative   bool
		wantFreeForm bool
	}{
		{
			name: "built-in OpenAI with SDK default URL",
			provider: catalog.Provider{
				Code: "openai", Type: catalog.APIOpenAI, APIKey: "test-key", Official: true, FreeFormTool: true,
			},
			wantNative: true, wantFreeForm: true,
		},
		{
			name: "built-in OpenAI with official URL",
			provider: catalog.Provider{
				Code: "openai", Type: catalog.APIOpenAI, APIKey: "test-key", Official: true,
				BaseURL: "https://api.openai.com/v1/",
			},
			wantNative: true,
		},
		{
			name: "OpenRouter Responses compatibility",
			provider: catalog.Provider{
				Code: "openrouter", Type: catalog.APIOpenAI, APIKey: "test-key",
				BaseURL: "https://openrouter.ai/api/v1",
			},
		},
		{
			name: "custom proxy using OpenAI code",
			provider: catalog.Provider{
				Code: "openai", Type: catalog.APIOpenAI, APIKey: "test-key",
				BaseURL: "https://proxy.example/v1",
			},
		},
		{
			name: "custom provider using official endpoint",
			provider: catalog.Provider{
				Code: "custom-openai", Type: catalog.APIOpenAI, APIKey: "test-key",
				BaseURL: "https://api.openai.com/v1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			caller, err := NewCaller(t.Context(), tt.provider, testModel())
			if err != nil {
				t.Fatalf("NewCaller() error = %v", err)
			}
			responsesCaller, ok := caller.(*openAIResponsesCaller)
			if !ok {
				t.Fatalf("NewCaller() = %T, want *openAIResponsesCaller", caller)
			}
			if responsesCaller.nativeOpenAI != tt.wantNative {
				t.Errorf("nativeOpenAI = %v, want %v", responsesCaller.nativeOpenAI, tt.wantNative)
			}
			if responsesCaller.freeFormTool != tt.wantFreeForm {
				t.Errorf("freeFormTool = %v, want %v", responsesCaller.freeFormTool, tt.wantFreeForm)
			}
		})
	}
}

func TestNativeOpenAIResponsesProviderRequiresOfficialResponsesAPI(t *testing.T) {
	if !nativeOpenAIResponsesProvider(catalog.Provider{Type: catalog.APIOpenAI, Official: true}) {
		t.Fatal("official OpenAI Responses provider was not recognized")
	}
	if nativeOpenAIResponsesProvider(catalog.Provider{Type: catalog.APIOpenAICompletions, Official: true}) {
		t.Fatal("official OpenAI Chat Completions provider was recognized as Responses")
	}
	if nativeOpenAIResponsesProvider(catalog.Provider{Type: catalog.APIOpenAI}) {
		t.Fatal("compatible Responses provider was recognized as official OpenAI")
	}
}

func testModel() catalog.Model {
	return catalog.Model{
		Code:             "test-model",
		ReasoningEfforts: shared.StandardReasoningEfforts(),
	}
}

func boolPointer(value bool) *bool {
	return &value
}

func testTool() modelToolDefinition {
	return modelToolDefinition{
		Name: "lookup", Description: "Look up a value",
		InputSchema: toolJSONSchema{
			Type: toolJSONSchemaTypeObject,
			Properties: map[string]toolJSONSchema{
				"q": {Type: toolJSONSchemaTypeString},
			},
			Required:             []string{"q"},
			AdditionalProperties: allowAdditionalProperties(false),
		},
		Strict: true,
	}
}

func testShellTool() modelToolDefinition {
	return modelToolDefinition{
		Type: agentloop.ToolTypeShell, Name: "shell", Description: "Execute shell commands",
		InputSchema: toolJSONSchema{
			Type: toolJSONSchemaTypeObject,
			Properties: map[string]toolJSONSchema{
				"commands": {
					Type:  toolJSONSchemaTypeArray,
					Items: &toolJSONSchema{Type: toolJSONSchemaTypeString},
				},
				"timeout_ms":        {Type: toolJSONSchemaTypeInteger},
				"max_output_length": {Type: toolJSONSchemaTypeInteger},
			},
			Required:             []string{"commands"},
			AdditionalProperties: allowAdditionalProperties(false),
		},
		Strict: false,
	}
}

func testApplyPatchTool() modelToolDefinition {
	tool := testNamedTool("apply_patch")
	tool.Type = agentloop.ToolTypeApplyPatch
	return tool
}

func testNamedTool(name string) modelToolDefinition {
	tool := testTool()
	tool.Name = name
	return tool
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

func assertShellToolSchemaMap(t *testing.T, schema map[string]any) {
	t.Helper()
	if schema["type"] != "object" {
		t.Errorf("type = %#v, want object", schema["type"])
	}
	if additional, ok := schema["additionalProperties"].(bool); !ok || additional {
		t.Errorf("additionalProperties = %#v, want false", schema["additionalProperties"])
	}
	required, ok := schema["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "commands" {
		t.Errorf("required = %#v, want [commands]", schema["required"])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v, want object", schema["properties"])
	}
	if _, ok := properties["commands"]; !ok {
		t.Errorf("properties = %#v, want commands", properties)
	}
	if _, ok := properties["action"]; ok {
		t.Errorf("properties = %#v, unexpected action wrapper", properties)
	}
}

func mustUnmarshal(t *testing.T, data string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(data), target); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
}

func assertResponse(t *testing.T, response *modelResponse, reason modelStopReason, contentLength int, total int64) {
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
