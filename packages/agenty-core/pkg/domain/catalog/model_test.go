package catalog

import (
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestModel_ReasoningEffortMapping(t *testing.T) {
	t.Parallel()

	model := Model{ReasoningEffortMapping: map[string]shared.ReasoningEffort{
		"none":    shared.ReasoningOff,
		"minimal": shared.ReasoningLow,
		"low":     shared.ReasoningLow,
		"high":    shared.ReasoningHigh,
	}}

	tests := []struct {
		name         string
		nativeEffort string
		want         shared.ReasoningEffort
		wantOK       bool
	}{
		{name: "off", nativeEffort: "none", want: shared.ReasoningOff, wantOK: true},
		{name: "first low alias", nativeEffort: "minimal", want: shared.ReasoningLow, wantOK: true},
		{name: "second low alias", nativeEffort: "low", want: shared.ReasoningLow, wantOK: true},
		{name: "missing", nativeEffort: "xhigh", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := model.MapReasoningEffort(tt.nativeEffort)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("MapReasoningEffort(%q) = %q, %v; want %q, %v", tt.nativeEffort, got, ok, tt.want, tt.wantOK)
			}
		})
	}

	if !model.SupportsReasoning() {
		t.Error("SupportsReasoning() = false, want true")
	}
	if !model.SupportsReasoningEffort(shared.ReasoningLow) {
		t.Error("SupportsReasoningEffort(low) = false, want true")
	}
	if model.SupportsReasoningEffort(shared.ReasoningMax) {
		t.Error("SupportsReasoningEffort(max) = true, want false")
	}
}

func TestModel_SupportsReasoningRequiresEnabledEffort(t *testing.T) {
	t.Parallel()

	model := Model{ReasoningEffortMapping: map[string]shared.ReasoningEffort{
		"none": shared.ReasoningOff,
	}}
	if model.SupportsReasoning() {
		t.Error("SupportsReasoning() = true for an off-only mapping")
	}
}
