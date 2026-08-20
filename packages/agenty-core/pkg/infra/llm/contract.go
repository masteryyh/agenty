package llm

import (
	"fmt"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
)

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

func providerToolType(tool modelToolDefinition) (agentloop.ToolType, error) {
	if tool.Type == "" {
		return agentloop.ToolTypeFunction, nil
	}
	switch tool.Type {
	case agentloop.ToolTypeFunction, agentloop.ToolTypeShell:
		return tool.Type, nil
	default:
		return "", invalidRequest("tool %q has unsupported type %q", tool.Name, tool.Type)
	}
}
