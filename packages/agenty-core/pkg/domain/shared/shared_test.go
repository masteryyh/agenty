package shared

import "testing"

func TestNewSlug(t *testing.T) {
	valid := []string{
		"anthropic",
		"claude-opus",
		"claude-opus-4",
		"claude-opus-4-8",
		"claude-sonnet-5",
		"claude-haiku-4-5",
		"gpt-5",
		"gpt-5.6",
		"gemini-3.5-flash",
		"kimi-k3",
		"kimi-k2.6",
		"deepseek-v4-pro",
		"qwen3.7-max",
		"glm-5.2",
		"glm-4.5-air",
		"claude_opus",
		"o1-mini",
		"a",
		"hello-world-123",
	}
	for _, s := range valid {
		if _, err := NewSlug(s); err != nil {
			t.Errorf("NewSlug(%q) returned error: %v", s, err)
		}
	}

	invalid := []string{
		"",
		"Anthropic",
		"claude--opus",
		"a..b",
		"foo/bar",
		".hidden",
		"-leading",
		"trailing-",
		"has space",
		"有中文",
		"4o-mini",
		"foo__bar",
	}
	for _, s := range invalid {
		if _, err := NewSlug(s); err == nil {
			t.Errorf("NewSlug(%q) expected error, got nil", s)
		}
	}
}

func TestThinkingEffortValid(t *testing.T) {
	for _, e := range []ThinkingEffort{ThinkingOff, ThinkingLow, ThinkingMedium, ThinkingHigh, ThinkingXHigh, ThinkingMax} {
		if !e.Valid() {
			t.Errorf("expected %q to be valid", e)
		}
	}
	if ThinkingEffort("insane").Valid() {
		t.Error("expected unknown effort to be invalid")
	}
	if ThinkingOff.Enabled() {
		t.Error("expected ThinkingOff to be disabled")
	}
	if !ThinkingHigh.Enabled() {
		t.Error("expected ThinkingHigh to be enabled")
	}
}

func TestModelRef(t *testing.T) {
	ref := NewModelRef("anthropic", "claude-opus-4")
	if ref.IsZero() {
		t.Error("expected non-zero ref")
	}
	if got, want := ref.String(), "anthropic/claude-opus-4"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if !(ModelRef{}).IsZero() {
		t.Error("expected empty ref to be zero")
	}
}
