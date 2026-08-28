package shared

import "fmt"

type ReasoningEffort string

const (
	ReasoningOff    ReasoningEffort = "off"
	ReasoningLow    ReasoningEffort = "low"
	ReasoningMedium ReasoningEffort = "medium"
	ReasoningHigh   ReasoningEffort = "high"
	ReasoningXHigh  ReasoningEffort = "xhigh"
	ReasoningMax    ReasoningEffort = "max"
)

func (r ReasoningEffort) Valid() bool {
	switch r {
	case ReasoningOff, ReasoningLow, ReasoningMedium, ReasoningHigh, ReasoningXHigh, ReasoningMax:
		return true
	default:
		return false
	}
}

func (r ReasoningEffort) Enabled() bool {
	return r != "" && r != ReasoningOff
}

func StandardReasoningEfforts() []ReasoningEffort {
	return []ReasoningEffort{
		ReasoningLow,
		ReasoningMedium,
		ReasoningHigh,
		ReasoningXHigh,
		ReasoningMax,
	}
}

// NormalizeReasoningEfforts keeps supported Agenty levels in their canonical order.
// A nil input means the upstream did not report capabilities and receives the defaults;
// an explicit empty slice identifies a non-reasoning model.
func NormalizeReasoningEfforts(efforts []ReasoningEffort) []ReasoningEffort {
	if efforts == nil {
		return StandardReasoningEfforts()
	}

	normalized := make([]ReasoningEffort, 0, len(efforts))
	for _, supported := range StandardReasoningEfforts() {
		for _, effort := range efforts {
			if effort == supported {
				normalized = append(normalized, supported)
				break
			}
		}
	}
	return normalized
}

func IsStandardReasoningEffort(effort ReasoningEffort) bool {
	for _, supported := range StandardReasoningEfforts() {
		if supported == effort {
			return true
		}
	}
	return false
}

func ValidateReasoningEfforts(efforts []ReasoningEffort) error {
	seen := make(map[ReasoningEffort]struct{}, len(efforts))
	for _, effort := range efforts {
		if !IsStandardReasoningEffort(effort) {
			return fmt.Errorf("unsupported reasoning effort %q", effort)
		}
		if _, ok := seen[effort]; ok {
			return fmt.Errorf("duplicate reasoning effort %q", effort)
		}
		seen[effort] = struct{}{}
	}
	return nil
}
