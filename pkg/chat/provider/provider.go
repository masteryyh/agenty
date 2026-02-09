package provider

import (
	"context"

	"github.com/masteryyh/agenty/pkg/chat/tools"
)

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
	RoleSystem    = "system"
)

type Message struct {
	Role       string            `json:"role"`
	Content    string            `json:"content"`
	ToolCalls  []tools.ToolCall  `json:"tool_calls,omitempty"`
	ToolResult *tools.ToolResult `json:"tool_result,omitempty"`
}

type ChatRequest struct {
	Model     string
	Messages  []Message
	Tools     []tools.ToolDefinition
	BaseURL   string
	APIKey    string
	MaxTokens int64
}

type ChatResponse struct {
	Content    string
	ToolCalls  []tools.ToolCall
	TotalToken int64
}

type ChatProvider interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	Name() string
}
