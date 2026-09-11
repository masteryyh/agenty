package agentloop

import "github.com/masteryyh/agenty-core/pkg/domain/conversation"

// ToolExecutionError lets a tool report structured output together with a
// failure. The tool registry preserves that output and marks the result as an
// error for the model.
type ToolExecutionError struct {
	Content conversation.Content
	Err     error
}

func (err *ToolExecutionError) Error() string {
	if err == nil || err.Err == nil {
		return "tool execution failed"
	}
	return err.Err.Error()
}

func (err *ToolExecutionError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}
