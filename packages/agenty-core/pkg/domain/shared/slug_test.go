package shared

import "testing"

func TestNewSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "plain", value: "anthropic", valid: true},
		{name: "hyphenated model", value: "claude-opus-4-8", valid: true},
		{name: "dot model", value: "gpt-5.6", valid: true},
		{name: "underscore model", value: "o1_mini", valid: true},
		{name: "letter followed by digit", value: "qwen3.7-max", valid: true},
		{name: "empty", value: "", valid: false},
		{name: "uppercase", value: "Anthropic", valid: false},
		{name: "repeated separator", value: "a..b", valid: false},
		{name: "path separator", value: "foo/bar", valid: false},
		{name: "hidden path", value: ".hidden", valid: false},
		{name: "leading digit", value: "4o-mini", valid: false},
		{name: "trailing separator", value: "model-", valid: false},
		{name: "whitespace", value: "has space", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			slug, err := NewSlug(tt.value)
			if tt.valid {
				if err != nil {
					t.Fatalf("NewSlug(%q): %v", tt.value, err)
				}
				if !slug.Valid() {
					t.Errorf("Slug(%q).Valid() = false", tt.value)
				}
				return
			}
			if err == nil {
				t.Errorf("NewSlug(%q) succeeded, want error", tt.value)
			}
		})
	}
}
