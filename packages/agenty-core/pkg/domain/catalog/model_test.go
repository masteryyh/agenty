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
