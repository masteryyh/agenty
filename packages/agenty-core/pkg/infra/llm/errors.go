package llm

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidRequest     = errors.New("llm: invalid request")
	ErrUnsupportedAPI     = errors.New("llm: unsupported API type")
	ErrUnsupportedContent = errors.New("llm: unsupported content")
)

func invalidRequest(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRequest, fmt.Sprintf(format, args...))
}

func unsupportedContent(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUnsupportedContent, fmt.Sprintf(format, args...))
}
