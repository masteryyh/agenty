package shared

type ThinkingEffort string

const (
	ThinkingOff     ThinkingEffort = "off"
	ThinkingMinimal ThinkingEffort = "minimal"
	ThinkingLow     ThinkingEffort = "low"
	ThinkingMedium  ThinkingEffort = "medium"
	ThinkingHigh    ThinkingEffort = "high"
	ThinkingXHigh   ThinkingEffort = "xhigh"
	ThinkingMax     ThinkingEffort = "max"
)

func (t ThinkingEffort) Valid() bool {
	switch t {
	case ThinkingOff, ThinkingMinimal, ThinkingLow, ThinkingMedium, ThinkingHigh, ThinkingXHigh, ThinkingMax:
		return true
	default:
		return false
	}
}

func (t ThinkingEffort) Enabled() bool {
	return t != "" && t != ThinkingOff
}
