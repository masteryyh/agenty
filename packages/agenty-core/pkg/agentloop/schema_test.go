package agentloop

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	json "github.com/bytedance/sonic"
)

func TestJSONSchemaToolDefinitionJSONRoundTrip(t *testing.T) {
	t.Parallel()

	const fixture = `{
		"name":"lookup",
		"description":"Look up a value",
		"inputSchema":{
			"type":"object",
			"properties":{
				"q":{"type":"string","minLength":1},
				"limit":{"type":"integer","minimum":1}
			},
			"required":["q"],
			"additionalProperties":false
		},
		"strict":true
	}`

	var tool ToolDefinition
	if err := json.Unmarshal([]byte(fixture), &tool); err != nil {
		t.Fatalf("unmarshal ToolDefinition: %v", err)
	}
	if tool.InputSchema.Type != JSONSchemaTypeObject {
		t.Errorf("schema type = %q, want object", tool.InputSchema.Type)
	}
	if tool.InputSchema.Properties["q"].Type != JSONSchemaTypeString {
		t.Errorf("q schema = %#v", tool.InputSchema.Properties["q"])
	}
	if tool.InputSchema.Properties["limit"].Minimum == nil ||
		*tool.InputSchema.Properties["limit"].Minimum != 1 {
		t.Errorf("limit schema = %#v", tool.InputSchema.Properties["limit"])
	}
	additional := tool.InputSchema.AdditionalProperties
	if additional == nil || additional.Allowed == nil || *additional.Allowed {
		t.Errorf("additionalProperties = %#v, want false", additional)
	}

	encoded, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("marshal ToolDefinition: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatalf("unmarshal ToolDefinition wire JSON: %v", err)
	}
	inputSchema, ok := wire["inputSchema"].(map[string]any)
	if !ok {
		t.Fatalf("inputSchema = %#v, want object", wire["inputSchema"])
	}
	if inputSchema["additionalProperties"] != false {
		t.Errorf("additionalProperties = %#v, want false", inputSchema["additionalProperties"])
	}
}

func TestJSONSchemaToMapPreservesRecursiveKeywords(t *testing.T) {
	t.Parallel()

	schema := JSONSchema{
		Type: JSONSchemaTypeObject,
		Properties: map[string]JSONSchema{
			"tags": {
				Type:  JSONSchemaTypeArray,
				Items: &JSONSchema{Type: JSONSchemaTypeString},
			},
			"metadata": {
				Type: JSONSchemaTypeObject,
				AdditionalProperties: AdditionalPropertiesSchema(JSONSchema{
					Type: JSONSchemaTypeString,
				}),
			},
		},
		Required: []string{"tags"},
		OneOf: []JSONSchema{
			{Required: []string{"tags"}},
			{Required: []string{"metadata"}},
		},
		AdditionalProperties: AllowAdditionalProperties(false),
	}

	converted, err := ToolSchemaMap(schema)
	if err != nil {
		t.Fatalf("convert schema: %v", err)
	}
	properties := converted["properties"].(map[string]any)
	tags := properties["tags"].(map[string]any)
	items := tags["items"].(map[string]any)
	if items["type"] != "string" {
		t.Errorf("items = %#v, want string schema", items)
	}
	metadata := properties["metadata"].(map[string]any)
	additional := metadata["additionalProperties"].(map[string]any)
	if additional["type"] != "string" {
		t.Errorf("metadata additionalProperties = %#v", additional)
	}
	if oneOf, ok := converted["oneOf"].([]any); !ok || len(oneOf) != 2 {
		t.Errorf("oneOf = %#v, want two schemas", converted["oneOf"])
	}
}

func TestJSONSchemaAdditionalPropertiesRejectsInvalidState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		properties JSONSchemaAdditionalProperties
	}{
		{
			name: "empty union",
		},
		{
			name: "boolean and schema",
			properties: JSONSchemaAdditionalProperties{
				Allowed: schemaPointer(false),
				Schema:  &JSONSchema{Type: JSONSchemaTypeString},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := json.Marshal(tt.properties)
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("Marshal() error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestJSONSchemaAdditionalPropertiesHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		properties *JSONSchemaAdditionalProperties
		wantJSON   string
	}{
		{
			name:       "allow",
			properties: AllowAdditionalProperties(true),
			wantJSON:   `true`,
		},
		{
			name:       "deny",
			properties: AllowAdditionalProperties(false),
			wantJSON:   `false`,
		},
		{
			name: "schema",
			properties: AdditionalPropertiesSchema(JSONSchema{
				Type:      JSONSchemaTypeString,
				MinLength: schemaPointer(uint64(1)),
			}),
			wantJSON: `{"type":"string","minLength":1}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := json.Marshal(tt.properties)
			if err != nil {
				t.Fatalf("Marshal() error: %v", err)
			}
			assertJSONEquivalent(t, []byte(tt.wantJSON), encoded)
		})
	}
}

func TestJSONSchemaAdditionalPropertiesJSONRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fixture string
		check   func(*testing.T, JSONSchemaAdditionalProperties)
	}{
		{
			name:    "true",
			fixture: `true`,
			check: func(t *testing.T, properties JSONSchemaAdditionalProperties) {
				t.Helper()
				if properties.Allowed == nil || !*properties.Allowed || properties.Schema != nil {
					t.Errorf("properties = %#v, want allowed true", properties)
				}
			},
		},
		{
			name:    "false with whitespace",
			fixture: "  false\n",
			check: func(t *testing.T, properties JSONSchemaAdditionalProperties) {
				t.Helper()
				if properties.Allowed == nil || *properties.Allowed || properties.Schema != nil {
					t.Errorf("properties = %#v, want allowed false", properties)
				}
			},
		},
		{
			name:    "empty schema",
			fixture: `{}`,
			check: func(t *testing.T, properties JSONSchemaAdditionalProperties) {
				t.Helper()
				if properties.Allowed != nil || properties.Schema == nil {
					t.Errorf("properties = %#v, want empty schema", properties)
				}
			},
		},
		{
			name:    "recursive schema",
			fixture: `{"type":"array","items":{"type":"integer","minimum":0}}`,
			check: func(t *testing.T, properties JSONSchemaAdditionalProperties) {
				t.Helper()
				if properties.Schema == nil || properties.Schema.Items == nil {
					t.Fatalf("properties = %#v, want recursive schema", properties)
				}
				if properties.Schema.Items.Minimum == nil || *properties.Schema.Items.Minimum != 0 {
					t.Errorf("items = %#v, want minimum 0", properties.Schema.Items)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var properties JSONSchemaAdditionalProperties
			if err := json.Unmarshal([]byte(tt.fixture), &properties); err != nil {
				t.Fatalf("Unmarshal() error: %v", err)
			}
			tt.check(t, properties)

			encoded, err := json.Marshal(properties)
			if err != nil {
				t.Fatalf("Marshal() error: %v", err)
			}
			assertJSONEquivalent(t, []byte(tt.fixture), encoded)
		})
	}
}

func TestJSONSchemaAdditionalPropertiesUnmarshalRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fixture     string
		wantMessage string
	}{
		{
			name:        "empty",
			fixture:     "",
			wantMessage: "must not be empty",
		},
		{
			name:        "whitespace",
			fixture:     " \n\t ",
			wantMessage: "must not be empty",
		},
		{
			name:        "null",
			fixture:     `null`,
			wantMessage: "must be a boolean or schema object",
		},
		{
			name:        "array",
			fixture:     `[]`,
			wantMessage: "must be a boolean or schema object",
		},
		{
			name:        "string",
			fixture:     `"object"`,
			wantMessage: "must be a boolean or schema object",
		},
		{
			name:        "number",
			fixture:     `1`,
			wantMessage: "must be a boolean or schema object",
		},
		{
			name:        "malformed object",
			fixture:     `{"type":`,
			wantMessage: "decode additionalProperties schema",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var properties JSONSchemaAdditionalProperties
			err := properties.UnmarshalJSON([]byte(tt.fixture))
			if err == nil || !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("UnmarshalJSON() error = %v, want containing %q", err, tt.wantMessage)
			}
		})
	}
}

func TestJSONSchemaAdditionalPropertiesUnmarshalResetsState(t *testing.T) {
	t.Parallel()

	properties := JSONSchemaAdditionalProperties{
		Allowed: schemaPointer(true),
	}
	if err := json.Unmarshal([]byte(`{"type":"integer"}`), &properties); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	if properties.Allowed != nil || properties.Schema == nil {
		t.Fatalf("properties = %#v, want schema only", properties)
	}

	if err := json.Unmarshal([]byte(`false`), &properties); err != nil {
		t.Fatalf("unmarshal boolean: %v", err)
	}
	if properties.Allowed == nil || *properties.Allowed || properties.Schema != nil {
		t.Fatalf("properties = %#v, want allowed false only", properties)
	}
}

func TestToolSchemaMapDefaultsAndValidatesType(t *testing.T) {
	t.Parallel()

	t.Run("defaults missing type to object without mutating input", func(t *testing.T) {
		t.Parallel()

		schema := JSONSchema{Description: "parameters"}
		converted, err := ToolSchemaMap(schema)
		if err != nil {
			t.Fatalf("toolSchemaMap() error: %v", err)
		}
		if converted["type"] != "object" {
			t.Errorf("type = %#v, want object", converted["type"])
		}
		if schema.Type != "" {
			t.Errorf("input type = %q, want unchanged", schema.Type)
		}
	})

	invalidTypes := []JSONSchemaType{
		JSONSchemaTypeArray,
		JSONSchemaTypeString,
		JSONSchemaTypeNumber,
		JSONSchemaTypeInteger,
		JSONSchemaTypeBoolean,
		JSONSchemaTypeNull,
		JSONSchemaType("custom"),
	}
	for _, schemaType := range invalidTypes {
		t.Run(string(schemaType), func(t *testing.T) {
			t.Parallel()

			_, err := ToolSchemaMap(JSONSchema{Type: schemaType})
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("toolSchemaMap() error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestJSONSchemaToMapMatchesJSONContract(t *testing.T) {
	t.Parallel()

	stringSchema := JSONSchema{
		Type:      JSONSchemaTypeString,
		MinLength: schemaPointer(uint64(0)),
	}
	schema := JSONSchema{
		Version:    "https://json-schema.org/draft/2020-12/schema",
		ID:         "https://example.com/tool.schema.json",
		Anchor:     "tool",
		Ref:        "#/$defs/value",
		DynamicRef: "#value",
		Definitions: map[string]JSONSchema{
			"value": stringSchema,
		},
		Comment: "complete schema",
		AllOf:   []JSONSchema{{Title: "all"}},
		AnyOf:   []JSONSchema{{Title: "any"}},
		OneOf:   []JSONSchema{{Title: "one"}},
		Not:     &JSONSchema{Type: JSONSchemaTypeNull},
		If:      &JSONSchema{Required: []string{"enabled"}},
		Then:    &JSONSchema{Properties: map[string]JSONSchema{"mode": stringSchema}},
		Else:    &JSONSchema{Properties: map[string]JSONSchema{"fallback": stringSchema}},
		DependentSchemas: map[string]JSONSchema{
			"mode": {Required: []string{"enabled"}},
		},
		PrefixItems: []JSONSchema{{Type: JSONSchemaTypeString}},
		Items:       &JSONSchema{Type: JSONSchemaTypeInteger},
		Contains:    &JSONSchema{Const: "match"},
		Properties: map[string]JSONSchema{
			"name": stringSchema,
		},
		PatternProperties: map[string]JSONSchema{
			"^x-": stringSchema,
		},
		AdditionalProperties: AllowAdditionalProperties(false),
		PropertyNames:        &JSONSchema{Pattern: "^[a-z]+$"},
		Type:                 JSONSchemaTypeObject,
		Enum:                 []any{"first", float64(2), false},
		Const:                false,
		MultipleOf:           schemaPointer(0.5),
		Maximum:              schemaPointer(100.0),
		ExclusiveMaximum:     schemaPointer(101.0),
		Minimum:              schemaPointer(0.0),
		ExclusiveMinimum:     schemaPointer(-1.0),
		MaxLength:            schemaPointer(uint64(200)),
		MinLength:            schemaPointer(uint64(0)),
		Pattern:              "^[a-z]+$",
		MaxItems:             schemaPointer(uint64(10)),
		MinItems:             schemaPointer(uint64(0)),
		UniqueItems:          true,
		MaxContains:          schemaPointer(uint64(5)),
		MinContains:          schemaPointer(uint64(0)),
		MaxProperties:        schemaPointer(uint64(20)),
		MinProperties:        schemaPointer(uint64(0)),
		Required:             []string{"name"},
		DependentRequired: map[string][]string{
			"name": {"enabled"},
		},
		Format:           "custom",
		ContentEncoding:  "base64",
		ContentMediaType: "application/json",
		ContentSchema:    &JSONSchema{Type: JSONSchemaTypeObject},
		Title:            "Tool input",
		Description:      "Complete conversion contract",
		Default:          map[string]any{"name": "default"},
		Deprecated:       true,
		ReadOnly:         true,
		WriteOnly:        true,
		Examples:         []any{map[string]any{"name": "example"}},
	}

	want, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema contract: %v", err)
	}
	converted, err := schema.toMap()
	if err != nil {
		t.Fatalf("toMap() error: %v", err)
	}
	got, err := json.Marshal(converted)
	if err != nil {
		t.Fatalf("marshal converted schema: %v", err)
	}

	assertJSONEquivalent(t, want, got)
}

func TestJSONSchemaToMapPropagatesNestedErrors(t *testing.T) {
	t.Parallel()

	invalid := JSONSchema{
		AdditionalProperties: &JSONSchemaAdditionalProperties{},
	}
	tests := []struct {
		name        string
		schema      JSONSchema
		wantContext string
	}{
		{name: "definitions", schema: JSONSchema{Definitions: map[string]JSONSchema{"bad": invalid}}, wantContext: "definitions"},
		{name: "allOf", schema: JSONSchema{AllOf: []JSONSchema{invalid}}, wantContext: "allOf"},
		{name: "anyOf", schema: JSONSchema{AnyOf: []JSONSchema{invalid}}, wantContext: "anyOf"},
		{name: "oneOf", schema: JSONSchema{OneOf: []JSONSchema{invalid}}, wantContext: "oneOf"},
		{name: "not", schema: JSONSchema{Not: &invalid}, wantContext: "not schema"},
		{name: "if", schema: JSONSchema{If: &invalid}, wantContext: "if schema"},
		{name: "then", schema: JSONSchema{Then: &invalid}, wantContext: "then schema"},
		{name: "else", schema: JSONSchema{Else: &invalid}, wantContext: "else schema"},
		{name: "dependentSchemas", schema: JSONSchema{DependentSchemas: map[string]JSONSchema{"bad": invalid}}, wantContext: "dependent schemas"},
		{name: "prefixItems", schema: JSONSchema{PrefixItems: []JSONSchema{invalid}}, wantContext: "prefix items"},
		{name: "items", schema: JSONSchema{Items: &invalid}, wantContext: "items schema"},
		{name: "contains", schema: JSONSchema{Contains: &invalid}, wantContext: "contains schema"},
		{name: "properties", schema: JSONSchema{Properties: map[string]JSONSchema{"bad": invalid}}, wantContext: "properties"},
		{name: "patternProperties", schema: JSONSchema{PatternProperties: map[string]JSONSchema{"bad": invalid}}, wantContext: "pattern properties"},
		{name: "additionalProperties", schema: invalid, wantContext: "additionalProperties"},
		{name: "propertyNames", schema: JSONSchema{PropertyNames: &invalid}, wantContext: "propertyNames schema"},
		{name: "contentSchema", schema: JSONSchema{ContentSchema: &invalid}, wantContext: "content schema"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tt.schema.toMap()
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("toMap() error = %v, want ErrInvalidRequest", err)
			}
			if !strings.Contains(err.Error(), tt.wantContext) {
				t.Errorf("toMap() error = %q, want context %q", err, tt.wantContext)
			}
		})
	}
}

func assertJSONEquivalent(t *testing.T, want, got []byte) {
	t.Helper()

	var wantValue any
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("unmarshal expected JSON %q: %v", want, err)
	}
	var gotValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("unmarshal actual JSON %q: %v", got, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("JSON mismatch\n got: %s\nwant: %s", got, want)
	}
}

func schemaPointer[T any](value T) *T {
	return &value
}
