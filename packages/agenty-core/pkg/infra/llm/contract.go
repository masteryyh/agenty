package llm

import (
	"fmt"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
)

// The agent loop owns the provider-neutral model contract. These aliases keep
// the provider adapters focused on conversion while preserving one canonical
// set of request, response, streaming, and schema types.
type (
	toolJSONSchema                     = agentloop.JSONSchema
	toolJSONSchemaAdditionalProperties = agentloop.JSONSchemaAdditionalProperties
	toolJSONSchemaType                 = agentloop.JSONSchemaType
	modelRequest                       = agentloop.Request
	modelResponse                      = agentloop.Response
	modelStopReason                    = agentloop.StopReason
	modelStreamEvent                   = agentloop.StreamEvent
	modelStreamEventType               = agentloop.StreamEventType
	modelStreamHandler                 = agentloop.StreamHandler
	modelToolDefinition                = agentloop.ToolDefinition
)

const (
	toolJSONSchemaTypeObject  = agentloop.JSONSchemaTypeObject
	toolJSONSchemaTypeArray   = agentloop.JSONSchemaTypeArray
	toolJSONSchemaTypeString  = agentloop.JSONSchemaTypeString
	toolJSONSchemaTypeNumber  = agentloop.JSONSchemaTypeNumber
	toolJSONSchemaTypeInteger = agentloop.JSONSchemaTypeInteger
	toolJSONSchemaTypeBoolean = agentloop.JSONSchemaTypeBoolean
	toolJSONSchemaTypeNull    = agentloop.JSONSchemaTypeNull

	modelStopReasonEndTurn       = agentloop.StopReasonEndTurn
	modelStopReasonMaxTokens     = agentloop.StopReasonMaxTokens
	modelStopReasonToolUse       = agentloop.StopReasonToolUse
	modelStopReasonContentFilter = agentloop.StopReasonContentFilter
	modelStopReasonError         = agentloop.StopReasonError

	modelStreamEventTextDelta      = agentloop.StreamEventTextDelta
	modelStreamEventReasoningDelta = agentloop.StreamEventReasoningDelta
	modelStreamEventToolUseStart   = agentloop.StreamEventToolUseStart
	modelStreamEventToolInputDelta = agentloop.StreamEventToolInputDelta
	modelStreamEventToolUseDone    = agentloop.StreamEventToolUseDone
	modelStreamEventCompleted      = agentloop.StreamEventCompleted
)

func allowAdditionalProperties(allowed bool) *toolJSONSchemaAdditionalProperties {
	return agentloop.AllowAdditionalProperties(allowed)
}

func additionalPropertiesSchema(schema toolJSONSchema) *toolJSONSchemaAdditionalProperties {
	return agentloop.AdditionalPropertiesSchema(schema)
}

func toolSchemaMap(schema toolJSONSchema) (map[string]any, error) {
	converted, err := agentloop.ToolSchemaMap(schema)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}

	return converted, nil
}
