package agentloop

import (
	"errors"
	"fmt"
)

var ErrInvalidRequest = errors.New("agentloop: invalid request")

func invalidRequest(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRequest, fmt.Sprintf(format, args...))
}
