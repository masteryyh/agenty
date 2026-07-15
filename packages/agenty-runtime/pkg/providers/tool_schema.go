/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package providers

import (
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/tools"
)

type ToolSchemaFamily string

const (
	ToolSchemaFamilyOpenAIResponses ToolSchemaFamily = "openai_responses"
	ToolSchemaFamilyOpenAIChat      ToolSchemaFamily = "openai_chat"
	ToolSchemaFamilyAnthropic       ToolSchemaFamily = "anthropic"
	ToolSchemaFamilyGemini          ToolSchemaFamily = "gemini"
)

func ToolSchemaFamilyForAPIType(apiType models.APIType) ToolSchemaFamily {
	switch apiType {
	case models.APITypeAnthropic:
		return ToolSchemaFamilyAnthropic
	case models.APITypeGemini:
		return ToolSchemaFamilyGemini
	case models.APITypeOpenAI, models.APITypeQwen:
		return ToolSchemaFamilyOpenAIResponses
	default:
		return ToolSchemaFamilyOpenAIChat
	}
}

func ToolSchemaForTokenEstimate(apiType models.APIType, defs []tools.ToolDefinition) any {
	switch ToolSchemaFamilyForAPIType(apiType) {
	case ToolSchemaFamilyOpenAIResponses:
		return openAIResponsesToolSchema(defs)
	case ToolSchemaFamilyAnthropic:
		return anthropicToolSchema(defs)
	case ToolSchemaFamilyGemini:
		return geminiToolSchema(defs)
	default:
		return openAIChatToolSchema(defs)
	}
}

func openAIResponsesToolSchema(defs []tools.ToolDefinition) []map[string]any {
	result := make([]map[string]any, 0, len(defs))
	for _, def := range defs {
		result = append(result, map[string]any{
			"type":        "function",
			"name":        def.Name,
			"description": def.Description,
			"parameters":  toolParametersSchema(def),
			"strict":      true,
		})
	}
	return result
}

func openAIChatToolSchema(defs []tools.ToolDefinition) []map[string]any {
	result := make([]map[string]any, 0, len(defs))
	for _, def := range defs {
		result = append(result, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        def.Name,
				"description": def.Description,
				"parameters":  toolParametersSchema(def),
			},
		})
	}
	return result
}

func anthropicToolSchema(defs []tools.ToolDefinition) []map[string]any {
	result := make([]map[string]any, 0, len(defs))
	for _, def := range defs {
		result = append(result, map[string]any{
			"name":         def.Name,
			"description":  def.Description,
			"input_schema": toolParametersSchema(def),
		})
	}
	return result
}

func geminiToolSchema(defs []tools.ToolDefinition) []map[string]any {
	functions := make([]map[string]any, 0, len(defs))
	for _, def := range defs {
		functions = append(functions, map[string]any{
			"name":        def.Name,
			"description": def.Description,
			"parameters":  geminiParametersSchema(def),
		})
	}
	return []map[string]any{{"functionDeclarations": functions}}
}

func toolParametersSchema(def tools.ToolDefinition) map[string]any {
	properties := make(map[string]any, len(def.Parameters.Properties))
	for name, prop := range def.Parameters.Properties {
		properties[name] = prop.ToMap()
	}
	return map[string]any{
		"type":       def.Parameters.Type,
		"properties": properties,
		"required":   def.Parameters.Required,
	}
}

func geminiParametersSchema(def tools.ToolDefinition) map[string]any {
	properties := make(map[string]any, len(def.Parameters.Properties))
	for name, prop := range def.Parameters.Properties {
		properties[name] = geminiPropertySchema(prop)
	}
	return map[string]any{
		"type":       def.Parameters.Type,
		"properties": properties,
		"required":   def.Parameters.Required,
	}
}

func geminiPropertySchema(prop tools.ParameterProperty) map[string]any {
	result := map[string]any{
		"type":        prop.Type,
		"description": prop.Description,
	}
	if prop.Items != nil {
		result["items"] = geminiPropertySchema(*prop.Items)
	}
	if len(prop.Properties) > 0 {
		properties := make(map[string]any, len(prop.Properties))
		for name, child := range prop.Properties {
			properties[name] = geminiPropertySchema(child)
		}
		result["properties"] = properties
	}
	if len(prop.Required) > 0 {
		result["required"] = prop.Required
	}
	return result
}
