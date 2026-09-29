package modelcall

import "fmt"

func toolSchemaMap(schema JSONSchema) (map[string]any, error) {
	converted, err := ToolSchemaMap(schema)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}

	return converted, nil
}

func providerToolType(tool ToolDefinition) (ToolType, error) {
	if tool.Type == "" {
		return ToolTypeFunction, nil
	}
	switch tool.Type {
	case ToolTypeFunction, ToolTypeShell, ToolTypeApplyPatch, ToolTypeTextEditor:
		return tool.Type, nil
	default:
		return "", invalidRequest("tool %q has unsupported type %q", tool.Name, tool.Type)
	}
}
