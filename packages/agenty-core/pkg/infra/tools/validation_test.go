package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
	infratools "github.com/masteryyh/agenty-core/pkg/infra/tools"
)

type validationRuntime struct {
	definitions []modelcall.ToolDefinition
}

func (runtime *validationRuntime) Definitions() []modelcall.ToolDefinition {
	return append([]modelcall.ToolDefinition(nil), runtime.definitions...)
}

func (*validationRuntime) ExecuteBatch(
	context.Context,
	agentloop.CallContext,
	[]conversation.ToolUseBlock,
) []conversation.ToolResultBlock {
	return nil
}

func TestValidationMiddlewareNormalizesMalformedModelToolInputs(t *testing.T) {
	t.Parallel()

	validation := infratools.NewValidationMiddleware()
	tests := []struct {
		name    string
		input   string
		invalid bool
	}{
		{name: "object", input: `{"path":"README.md"}`},
		{name: "truncated", input: `{"path"}`, invalid: true},
		{name: "array", input: `[]`, invalid: true},
		{name: "string", input: `"README.md"`, invalid: true},
		{name: "null", input: `null`, invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			response := &modelcall.ModelCallResponse{Content: conversation.Content{
				conversation.ToolUseBlock{ID: "call-1", Name: "read_file", Input: []byte(tt.input)},
			}}
			state := &middleware.ModelCallContext{Response: response}
			if err := validation.AfterModelCall(t.Context(), state); err != nil {
				t.Fatal(err)
			}

			call := response.Content[0].(conversation.ToolUseBlock)
			if tt.invalid {
				if string(call.Input) != `{}` || call.InputError == "" {
					t.Fatalf("normalized call = %#v", call)
				}
				return
			}
			if string(call.Input) != tt.input || call.InputError != "" {
				t.Fatalf("valid call changed = %#v", call)
			}
		})
	}
}

func TestValidationMiddlewareReturnsMalformedInputToModel(t *testing.T) {
	t.Parallel()

	validation := infratools.NewValidationMiddleware()
	response := &modelcall.ModelCallResponse{Content: conversation.Content{
		conversation.ToolUseBlock{ID: "call-1", Name: "read_file", Input: []byte(`{"path"}`)},
	}}
	if err := validation.AfterModelCall(t.Context(), &middleware.ModelCallContext{Response: response}); err != nil {
		t.Fatal(err)
	}

	call := response.Content[0].(conversation.ToolUseBlock)
	state := &middleware.ToolCallContext{
		Call: &call,
		Tools: &validationRuntime{definitions: []modelcall.ToolDefinition{{
			Type: modelcall.ToolTypeFunction,
			Name: "read_file",
		}}},
	}
	if err := validation.BeforeToolCall(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	if state.Result == nil || !state.Result.IsError || state.Result.ToolUseID != call.ID {
		t.Fatalf("validation result = %#v", state.Result)
	}
	message := state.Result.Content[0].(conversation.TextBlock).Text
	if !strings.Contains(message, "Invalid tool arguments: expected a complete JSON object.") ||
		!strings.Contains(message, "Parsing failed:") {
		t.Fatalf("validation message = %q", message)
	}
}

func TestValidationMiddlewareSkipsFreeFormTools(t *testing.T) {
	t.Parallel()

	validation := infratools.NewValidationMiddleware()
	response := &modelcall.ModelCallResponse{Content: conversation.Content{
		conversation.ApplyPatchCallBlock{
			CallID: "call-1", Source: conversation.ApplyPatchSourceCustom, Patch: "*** Begin Patch",
		},
	}}
	if err := validation.AfterModelCall(t.Context(), &middleware.ModelCallContext{Response: response}); err != nil {
		t.Fatal(err)
	}
	patch := response.Content[0].(conversation.ApplyPatchCallBlock)
	if patch.Patch != "*** Begin Patch" {
		t.Fatalf("free-form response changed = %#v", patch)
	}

	call := conversation.ToolUseBlock{
		ID: "call-1", Name: "apply_patch", Input: []byte("*** Begin Patch"),
	}
	state := &middleware.ToolCallContext{
		Call: &call,
		Tools: &validationRuntime{definitions: []modelcall.ToolDefinition{{
			Type: modelcall.ToolTypeApplyPatch,
			Name: "apply_patch",
		}}},
	}
	if err := validation.BeforeToolCall(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	if state.Result != nil || string(state.Call.Input) != "*** Begin Patch" {
		t.Fatalf("free-form call changed = %#v, result = %#v", state.Call, state.Result)
	}
}
