package catalog

import (
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestModelReasoningCapabilities(t *testing.T) {
	t.Parallel()

	model := Model{ReasoningEfforts: []shared.ReasoningEffort{
		shared.ReasoningLow,
		shared.ReasoningMedium,
		shared.ReasoningHigh,
	}}
	if !model.SupportsReasoning() {
		t.Fatal("SupportsReasoning() = false, want true")
	}
	if !model.SupportsReasoningEffort(shared.ReasoningHigh) {
		t.Fatal("SupportsReasoningEffort(high) = false, want true")
	}
	if model.SupportsReasoningEffort(shared.ReasoningXHigh) {
		t.Fatal("SupportsReasoningEffort(xhigh) = true, want false")
	}
}

func TestModelWithoutReasoningEffortsDoesNotSupportReasoning(t *testing.T) {
	t.Parallel()

	model := Model{ReasoningEfforts: []shared.ReasoningEffort{}}
	if model.SupportsReasoning() {
		t.Fatal("SupportsReasoning() = true, want false")
	}
}

func TestNormalizeReasoningCapabilitiesUsesDefaultsAndLegacyInference(t *testing.T) {
	defaultModel := Model{Reasoning: true}
	NormalizeReasoningCapabilities(&defaultModel)
	if len(defaultModel.ReasoningEfforts) != len(shared.StandardReasoningEfforts()) {
		t.Fatalf("default reasoning efforts = %v", defaultModel.ReasoningEfforts)
	}

	legacyModel := Model{ReasoningEfforts: []shared.ReasoningEffort{shared.ReasoningLow}}
	NormalizeReasoningCapabilities(&legacyModel)
	if !legacyModel.Reasoning || !legacyModel.SupportsReasoningEffort(shared.ReasoningLow) {
		t.Fatalf("legacy reasoning capabilities = %+v", legacyModel)
	}

	disabledModel := Model{Reasoning: false, ReasoningEfforts: []shared.ReasoningEffort{}}
	NormalizeReasoningCapabilities(&disabledModel)
	if disabledModel.Reasoning || len(disabledModel.ReasoningEfforts) != 0 {
		t.Fatalf("disabled reasoning capabilities = %+v", disabledModel)
	}
}
