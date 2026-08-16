package shared

import "testing"

func TestNewModelID(t *testing.T) {
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
		{name: "all requested separators", value: "org/model_name[v2]", valid: true},
		{name: "empty", value: "", valid: false},
		{name: "leading digit", value: "4o-mini", valid: false},
		{name: "leading slash", value: "/model", valid: false},
		{name: "trailing slash", value: "org/model/", valid: false},
		{name: "whitespace", value: "model name", valid: false},
		{name: "uppercase", value: "Model", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			modelID, err := NewModelID(tt.value)
			if tt.valid {
				if err != nil {
					t.Fatalf("NewModelID(%q): %v", tt.value, err)
				}
				if !modelID.Valid() {
					t.Errorf("ModelID(%q).Valid() = false", tt.value)
				}
				return
			}
			if err == nil {
				t.Errorf("NewModelID(%q) succeeded, want error", tt.value)
			}
		})
	}
}
