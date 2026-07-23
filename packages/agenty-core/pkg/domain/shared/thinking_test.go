package shared

import "testing"

func TestThinkingEffort_ValidAndEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		effort      ThinkingEffort
		valid       bool
		testEnabled bool
		wantEnabled bool
	}{
		{name: "empty", effort: "", valid: false, testEnabled: true, wantEnabled: false},
		{name: "off", effort: ThinkingOff, valid: true, testEnabled: true, wantEnabled: false},
		{name: "minimal", effort: ThinkingMinimal, valid: true, testEnabled: true, wantEnabled: true},
		{name: "low", effort: ThinkingLow, valid: true, testEnabled: true, wantEnabled: true},
		{name: "medium", effort: ThinkingMedium, valid: true, testEnabled: true, wantEnabled: true},
		{name: "high", effort: ThinkingHigh, valid: true, testEnabled: true, wantEnabled: true},
		{name: "xhigh", effort: ThinkingXHigh, valid: true, testEnabled: true, wantEnabled: true},
		{name: "max", effort: ThinkingMax, valid: true, testEnabled: true, wantEnabled: true},
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
