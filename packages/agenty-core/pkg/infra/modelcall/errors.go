package modelcall

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidRequest             = errors.New("modelcall: invalid request")
	ErrUnsupportedAPI             = errors.New("modelcall: unsupported API type")
	ErrUnsupportedContent         = errors.New("modelcall: unsupported content")
	ErrUnsupportedReasoningEffort = errors.New("modelcall: unsupported reasoning effort")
)

func invalidRequest(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRequest, fmt.Sprintf(format, args...))
}

func unsupportedContent(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUnsupportedContent, fmt.Sprintf(format, args...))
}
