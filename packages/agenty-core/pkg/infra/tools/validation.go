package tools

import (
	"context"
	"errors"
	"fmt"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
)

const invalidToolArgumentsMessage = "Invalid tool arguments: expected a complete JSON object. The tool was not executed. Retry the tool call with valid JSON arguments."

// NewValidationMiddleware turns malformed structured tool arguments into tool
// errors so the model can retry without ending the round.
func NewValidationMiddleware() middleware.Middleware {
	return middleware.Middleware{
		Name:           "tool-input-validation",
		AfterModelCall: validateModelToolInputs,
		BeforeToolCall: rejectInvalidToolInput,
	}
}

func validateModelToolInputs(_ context.Context, state *middleware.ModelCallContext) error {
	if state == nil || state.Response == nil {
		return nil
	}

	for index, block := range state.Response.Content {
		call, ok := block.(conversation.ToolUseBlock)
		if !ok {
			continue
		}
		if err := validateJSONObject(call.Input); err != nil {
			call.InputError = err.Error()
			call.Input = shared.RawJSON("{}")
			state.Response.Content[index] = call
		}
	}
	return nil
}

func rejectInvalidToolInput(_ context.Context, state *middleware.ToolCallContext) error {
	if state == nil || state.Call == nil || state.Result != nil {
		return nil
	}
	if freeFormTool(state.Tools, state.Call.Name) {
		return nil
	}

	inputError := state.Call.InputError
	if inputError == "" {
		if err := validateJSONObject(state.Call.Input); err != nil {
			inputError = err.Error()
			state.Call.InputError = inputError
			state.Call.Input = shared.RawJSON("{}")
		}
	}
	if inputError == "" {
		return nil
	}

	state.Result = &conversation.ToolResultBlock{
		ToolUseID: state.Call.ID,
		IsError:   true,
		Content: conversation.Text(fmt.Sprintf(
			"%s Parsing failed: %s",
			invalidToolArgumentsMessage,
			inputError,
		)),
	}
	return nil
}

func validateJSONObject(input shared.RawJSON) error {
	var value map[string]any
	if err := json.Unmarshal(input, &value); err != nil {
		return err
	}
	if value == nil {
		return errors.New("tool arguments must be a JSON object")
	}
	return nil
}

func freeFormTool(runtime agentloop.ToolRuntime, name string) bool {
	if runtime == nil {
		return false
	}
	for _, definition := range runtime.Definitions() {
		if definition.Name == name {
			return definition.Type == modelcall.ToolTypeApplyPatch
		}
	}
	return false
}
