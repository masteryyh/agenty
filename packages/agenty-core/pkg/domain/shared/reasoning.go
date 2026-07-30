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
