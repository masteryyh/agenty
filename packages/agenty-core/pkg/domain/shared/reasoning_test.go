package shared

import (
	"slices"
	"testing"
)

func TestReasoningEffort_ValidAndEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		effort      ReasoningEffort
		valid       bool
		testEnabled bool
		wantEnabled bool
	}{
		{name: "empty", effort: "", valid: false, testEnabled: true, wantEnabled: false},
		{name: "off", effort: ReasoningOff, valid: true, testEnabled: true, wantEnabled: false},
		{name: "low", effort: ReasoningLow, valid: true, testEnabled: true, wantEnabled: true},
		{name: "medium", effort: ReasoningMedium, valid: true, testEnabled: true, wantEnabled: true},
		{name: "high", effort: ReasoningHigh, valid: true, testEnabled: true, wantEnabled: true},
		{name: "xhigh", effort: ReasoningXHigh, valid: true, testEnabled: true, wantEnabled: true},
		{name: "max", effort: ReasoningMax, valid: true, testEnabled: true, wantEnabled: true},
		{name: "minimal", effort: "minimal", valid: false},
		{name: "unknown", effort: "extreme", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.effort.Valid(); got != tt.valid {
				t.Errorf("Valid() = %v, want %v", got, tt.valid)
			}
			if tt.testEnabled {
				if got := tt.effort.Enabled(); got != tt.wantEnabled {
					t.Errorf("Enabled() = %v, want %v", got, tt.wantEnabled)
				}
			}
		})
	}
}

func TestNormalizeReasoningEfforts(t *testing.T) {
	t.Parallel()

	wantDefault := []ReasoningEffort{
		ReasoningLow,
		ReasoningMedium,
		ReasoningHigh,
		ReasoningXHigh,
		ReasoningMax,
	}
	if got := NormalizeReasoningEfforts(nil); !slices.Equal(got, wantDefault) {
		t.Fatalf("NormalizeReasoningEfforts(nil) = %v, want %v", got, wantDefault)
	}
	if got := NormalizeReasoningEfforts([]ReasoningEffort{}); got == nil || len(got) != 0 {
		t.Fatalf("NormalizeReasoningEfforts(empty) = %v, want empty non-nil slice", got)
	}

	got := NormalizeReasoningEfforts([]ReasoningEffort{
		ReasoningMax,
		"minimal",
		ReasoningLow,
		ReasoningLow,
	})
	want := []ReasoningEffort{ReasoningLow, ReasoningMax}
	if !slices.Equal(got, want) {
		t.Fatalf("NormalizeReasoningEfforts(filtered) = %v, want %v", got, want)
	}
}
