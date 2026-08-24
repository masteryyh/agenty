package shared

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
