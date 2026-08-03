package llm

import (
	"bytes"
	"fmt"

	json "github.com/bytedance/sonic"
)

type JSONSchemaType string

const (
	JSONSchemaTypeObject  JSONSchemaType = "object"
	JSONSchemaTypeArray   JSONSchemaType = "array"
	JSONSchemaTypeString  JSONSchemaType = "string"
	JSONSchemaTypeNumber  JSONSchemaType = "number"
	JSONSchemaTypeInteger JSONSchemaType = "integer"
	JSONSchemaTypeBoolean JSONSchemaType = "boolean"
	JSONSchemaTypeNull    JSONSchemaType = "null"
)

type JSONSchema struct {
	Version     string                `json:"$schema,omitempty"`
	ID          string                `json:"$id,omitempty"`
	Anchor      string                `json:"$anchor,omitempty"`
	Ref         string                `json:"$ref,omitempty"`
	DynamicRef  string                `json:"$dynamicRef,omitempty"`
	Definitions map[string]JSONSchema `json:"$defs,omitempty"`
	Comment     string                `json:"$comment,omitempty"`

	AllOf []JSONSchema `json:"allOf,omitempty"`
	AnyOf []JSONSchema `json:"anyOf,omitempty"`
	OneOf []JSONSchema `json:"oneOf,omitempty"`
	Not   *JSONSchema  `json:"not,omitempty"`

	If               *JSONSchema           `json:"if,omitempty"`
	Then             *JSONSchema           `json:"then,omitempty"`
	Else             *JSONSchema           `json:"else,omitempty"`
	DependentSchemas map[string]JSONSchema `json:"dependentSchemas,omitempty"`

	PrefixItems []JSONSchema `json:"prefixItems,omitempty"`
	Items       *JSONSchema  `json:"items,omitempty"`
	Contains    *JSONSchema  `json:"contains,omitempty"`

	Properties           map[string]JSONSchema           `json:"properties,omitempty"`
	PatternProperties    map[string]JSONSchema           `json:"patternProperties,omitempty"`
	AdditionalProperties *JSONSchemaAdditionalProperties `json:"additionalProperties,omitempty"`
	PropertyNames        *JSONSchema                     `json:"propertyNames,omitempty"`

	Type              JSONSchemaType      `json:"type,omitempty"`
	Enum              []any               `json:"enum,omitempty"`
	Const             any                 `json:"const,omitempty"`
	MultipleOf        *float64            `json:"multipleOf,omitempty"`
	Maximum           *float64            `json:"maximum,omitempty"`
	ExclusiveMaximum  *float64            `json:"exclusiveMaximum,omitempty"`
	Minimum           *float64            `json:"minimum,omitempty"`
	ExclusiveMinimum  *float64            `json:"exclusiveMinimum,omitempty"`
	MaxLength         *uint64             `json:"maxLength,omitempty"`
	MinLength         *uint64             `json:"minLength,omitempty"`
	Pattern           string              `json:"pattern,omitempty"`
	MaxItems          *uint64             `json:"maxItems,omitempty"`
	MinItems          *uint64             `json:"minItems,omitempty"`
	UniqueItems       bool                `json:"uniqueItems,omitempty"`
	MaxContains       *uint64             `json:"maxContains,omitempty"`
	MinContains       *uint64             `json:"minContains,omitempty"`
	MaxProperties     *uint64             `json:"maxProperties,omitempty"`
	MinProperties     *uint64             `json:"minProperties,omitempty"`
	Required          []string            `json:"required,omitempty"`
	DependentRequired map[string][]string `json:"dependentRequired,omitempty"`

	Format           string      `json:"format,omitempty"`
	ContentEncoding  string      `json:"contentEncoding,omitempty"`
	ContentMediaType string      `json:"contentMediaType,omitempty"`
	ContentSchema    *JSONSchema `json:"contentSchema,omitempty"`

	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
	Deprecated  bool   `json:"deprecated,omitempty"`
	ReadOnly    bool   `json:"readOnly,omitempty"`
	WriteOnly   bool   `json:"writeOnly,omitempty"`
	Examples    []any  `json:"examples,omitempty"`
}

type JSONSchemaAdditionalProperties struct {
	Allowed *bool
	Schema  *JSONSchema
}

func AllowAdditionalProperties(allowed bool) *JSONSchemaAdditionalProperties {
	return &JSONSchemaAdditionalProperties{Allowed: &allowed}
}

func AdditionalPropertiesSchema(schema JSONSchema) *JSONSchemaAdditionalProperties {
	return &JSONSchemaAdditionalProperties{Schema: &schema}
}

func (properties JSONSchemaAdditionalProperties) MarshalJSON() ([]byte, error) {
	value, err := properties.value()
	if err != nil {
		return nil, err
	}

	return json.Marshal(value)
}

func (properties *JSONSchemaAdditionalProperties) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return fmt.Errorf("llm: additionalProperties must not be empty")
	}

	if bytes.Equal(trimmed, []byte("true")) || bytes.Equal(trimmed, []byte("false")) {
		var allowed bool
		if err := json.Unmarshal(trimmed, &allowed); err != nil {
			return fmt.Errorf("llm: decode additionalProperties boolean: %w", err)
		}
		properties.Allowed = &allowed
		properties.Schema = nil
		return nil
	}
	if trimmed[0] != '{' {
		return fmt.Errorf("llm: additionalProperties must be a boolean or schema object")
	}

	var schema JSONSchema
	if err := json.Unmarshal(trimmed, &schema); err != nil {
		return fmt.Errorf("llm: decode additionalProperties schema: %w", err)
	}
	properties.Allowed = nil
	properties.Schema = &schema

	return nil
}

func (properties JSONSchemaAdditionalProperties) value() (any, error) {
	if properties.Allowed != nil && properties.Schema != nil {
		return nil, invalidRequest("additionalProperties cannot contain both a boolean and a schema")
	}
	if properties.Allowed != nil {
		return *properties.Allowed, nil
	}
	if properties.Schema != nil {
		return properties.Schema.toMap()
	}

	return nil, invalidRequest("additionalProperties must contain a boolean or a schema")
}

func toolSchemaMap(schema JSONSchema) (map[string]any, error) {
	if schema.Type == "" {
		schema.Type = JSONSchemaTypeObject
	}
	if schema.Type != JSONSchemaTypeObject {
		return nil, invalidRequest(
			"tool input schema type must be %q, got %q",
			JSONSchemaTypeObject,
			schema.Type,
		)
	}

	return schema.toMap()
}

func (schema JSONSchema) toMap() (map[string]any, error) {
	result := make(map[string]any)
	setJSONSchemaMetadata(result, schema)

	if err := setJSONSchemaComposition(result, schema); err != nil {
		return nil, err
	}
	if err := setJSONSchemaArrays(result, schema); err != nil {
		return nil, err
	}
	if err := setJSONSchemaObjects(result, schema); err != nil {
		return nil, err
	}
	setJSONSchemaValidation(result, schema)

	if schema.ContentSchema != nil {
		contentSchema, err := schema.ContentSchema.toMap()
		if err != nil {
			return nil, fmt.Errorf("llm: convert content schema: %w", err)
		}
		result["contentSchema"] = contentSchema
	}

	return result, nil
}

func setJSONSchemaMetadata(result map[string]any, schema JSONSchema) {
	setString(result, "$schema", schema.Version)
	setString(result, "$id", schema.ID)
	setString(result, "$anchor", schema.Anchor)
	setString(result, "$ref", schema.Ref)
	setString(result, "$dynamicRef", schema.DynamicRef)
	setString(result, "$comment", schema.Comment)
	setString(result, "type", string(schema.Type))
	setString(result, "format", schema.Format)
	setString(result, "contentEncoding", schema.ContentEncoding)
	setString(result, "contentMediaType", schema.ContentMediaType)
	setString(result, "title", schema.Title)
	setString(result, "description", schema.Description)
	if schema.Const != nil {
		result["const"] = schema.Const
	}
	if schema.Default != nil {
		result["default"] = schema.Default
	}
	if len(schema.Enum) > 0 {
		result["enum"] = schema.Enum
	}
	if len(schema.Examples) > 0 {
		result["examples"] = schema.Examples
	}
	if schema.Deprecated {
		result["deprecated"] = true
	}
	if schema.ReadOnly {
		result["readOnly"] = true
	}
	if schema.WriteOnly {
		result["writeOnly"] = true
	}
}

func setJSONSchemaComposition(result map[string]any, schema JSONSchema) error {
	definitions, err := jsonSchemaMap(schema.Definitions)
	if err != nil {
		return fmt.Errorf("llm: convert schema definitions: %w", err)
	}
	setMap(result, "$defs", definitions)

	allOf, err := jsonSchemaList(schema.AllOf)
	if err != nil {
		return fmt.Errorf("llm: convert allOf: %w", err)
	}
	setList(result, "allOf", allOf)

	anyOf, err := jsonSchemaList(schema.AnyOf)
	if err != nil {
		return fmt.Errorf("llm: convert anyOf: %w", err)
	}
	setList(result, "anyOf", anyOf)

	oneOf, err := jsonSchemaList(schema.OneOf)
	if err != nil {
		return fmt.Errorf("llm: convert oneOf: %w", err)
	}
	setList(result, "oneOf", oneOf)

	if err := setSchema(result, "not", schema.Not); err != nil {
		return err
	}
	if err := setSchema(result, "if", schema.If); err != nil {
		return err
	}
	if err := setSchema(result, "then", schema.Then); err != nil {
		return err
	}
	if err := setSchema(result, "else", schema.Else); err != nil {
		return err
	}

	dependentSchemas, err := jsonSchemaMap(schema.DependentSchemas)
	if err != nil {
		return fmt.Errorf("llm: convert dependent schemas: %w", err)
	}
	setMap(result, "dependentSchemas", dependentSchemas)

	return nil
}

func setJSONSchemaArrays(result map[string]any, schema JSONSchema) error {
	prefixItems, err := jsonSchemaList(schema.PrefixItems)
	if err != nil {
		return fmt.Errorf("llm: convert prefix items: %w", err)
	}
	setList(result, "prefixItems", prefixItems)

	if err := setSchema(result, "items", schema.Items); err != nil {
		return err
	}
	if err := setSchema(result, "contains", schema.Contains); err != nil {
		return err
	}

	return nil
}

func setJSONSchemaObjects(result map[string]any, schema JSONSchema) error {
	properties, err := jsonSchemaMap(schema.Properties)
	if err != nil {
		return fmt.Errorf("llm: convert properties: %w", err)
	}
	setMap(result, "properties", properties)

	patternProperties, err := jsonSchemaMap(schema.PatternProperties)
	if err != nil {
		return fmt.Errorf("llm: convert pattern properties: %w", err)
	}
	setMap(result, "patternProperties", patternProperties)

	if schema.AdditionalProperties != nil {
		additionalProperties, err := schema.AdditionalProperties.value()
		if err != nil {
			return fmt.Errorf("llm: convert additionalProperties: %w", err)
		}
		result["additionalProperties"] = additionalProperties
	}
	if err := setSchema(result, "propertyNames", schema.PropertyNames); err != nil {
		return err
	}

	return nil
}

func setJSONSchemaValidation(result map[string]any, schema JSONSchema) {
	setFloat(result, "multipleOf", schema.MultipleOf)
	setFloat(result, "maximum", schema.Maximum)
	setFloat(result, "exclusiveMaximum", schema.ExclusiveMaximum)
	setFloat(result, "minimum", schema.Minimum)
	setFloat(result, "exclusiveMinimum", schema.ExclusiveMinimum)
	setUint(result, "maxLength", schema.MaxLength)
	setUint(result, "minLength", schema.MinLength)
	setString(result, "pattern", schema.Pattern)
	setUint(result, "maxItems", schema.MaxItems)
	setUint(result, "minItems", schema.MinItems)
	if schema.UniqueItems {
		result["uniqueItems"] = true
	}
	setUint(result, "maxContains", schema.MaxContains)
	setUint(result, "minContains", schema.MinContains)
	setUint(result, "maxProperties", schema.MaxProperties)
	setUint(result, "minProperties", schema.MinProperties)
	if len(schema.Required) > 0 {
		result["required"] = schema.Required
	}
	if len(schema.DependentRequired) > 0 {
		result["dependentRequired"] = schema.DependentRequired
	}
}

func jsonSchemaMap(values map[string]JSONSchema) (map[string]any, error) {
	result := make(map[string]any, len(values))
	for name, schema := range values {
		converted, err := schema.toMap()
		if err != nil {
			return nil, fmt.Errorf("schema %q: %w", name, err)
		}
		result[name] = converted
	}

	return result, nil
}

func jsonSchemaList(values []JSONSchema) ([]any, error) {
	result := make([]any, 0, len(values))
	for index, schema := range values {
		converted, err := schema.toMap()
		if err != nil {
			return nil, fmt.Errorf("schema %d: %w", index, err)
		}
		result = append(result, converted)
	}

	return result, nil
}

func setSchema(result map[string]any, key string, schema *JSONSchema) error {
	if schema == nil {
		return nil
	}

	converted, err := schema.toMap()
	if err != nil {
		return fmt.Errorf("llm: convert %s schema: %w", key, err)
	}
	result[key] = converted

	return nil
}

func setString(result map[string]any, key string, value string) {
	if value != "" {
		result[key] = value
	}
}

func setFloat(result map[string]any, key string, value *float64) {
	if value != nil {
		result[key] = *value
	}
}

func setUint(result map[string]any, key string, value *uint64) {
	if value != nil {
		result[key] = *value
	}
}

func setMap(result map[string]any, key string, value map[string]any) {
	if len(value) > 0 {
		result[key] = value
	}
}

func setList(result map[string]any, key string, value []any) {
	if len(value) > 0 {
		result[key] = value
	}
}
