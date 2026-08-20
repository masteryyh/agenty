package shared

import "testing"

func TestNewModelCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "plain", value: "gpt-5-mini", valid: true},
		{name: "underscore", value: "o1_mini", valid: true},
		{name: "namespace", value: "openai/gpt-oss", valid: true},
		{name: "variant brackets", value: "model[thinking]", valid: true},
		{name: "all requested separators", value: `org\\model_name:v2`, valid: true},
		{name: "uppercase provider code", value: "GPT-5.6", valid: true},
		{name: "empty", value: "", valid: false},
		{name: "surrounding whitespace", value: " model ", valid: false},
		{name: "control character", value: "model\nname", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			modelCode, err := NewModelCode(tt.value)
			if tt.valid {
				if err != nil {
					t.Fatalf("NewModelCode(%q): %v", tt.value, err)
				}
				if !modelCode.Valid() {
					t.Errorf("ModelCode(%q).Valid() = false", tt.value)
				}
				return
			}
			if err == nil {
				t.Errorf("NewModelCode(%q) succeeded, want error", tt.value)
			}
		})
	}
}
