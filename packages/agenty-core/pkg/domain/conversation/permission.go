package conversation

import "fmt"

// PermissionMode controls whether Agenty pauses before executing a tool.
type PermissionMode string

const (
	PermissionAsk  PermissionMode = "ask"
	PermissionAuto PermissionMode = "auto"
	PermissionYolo PermissionMode = "yolo"
)

func (mode PermissionMode) Valid() bool {
	return mode == PermissionAsk || mode == PermissionAuto || mode == PermissionYolo
}

func (mode PermissionMode) Normalized() PermissionMode {
	if mode.Valid() {
		return mode
	}
	return PermissionAsk
}

func ParsePermissionMode(value string) (PermissionMode, error) {
	mode := PermissionMode(value)
	if !mode.Valid() {
		return "", fmt.Errorf("invalid permission mode %q", value)
	}
	return mode, nil
}
