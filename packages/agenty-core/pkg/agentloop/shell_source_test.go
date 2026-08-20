package agentloop

import (
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

func TestMarkNativeShellResults(t *testing.T) {
	t.Parallel()

	calls := conversation.Content{
		conversation.ShellCallBlock{CallID: "native_call"},
		conversation.ToolUseBlock{ID: "function_call", Name: "shell"},
	}
	results := []conversation.ToolResultBlock{
		{
			ToolUseID: "native_call",
			Content:   conversation.Content{conversation.ShellCallOutputBlock{}},
		},
		{
			ToolUseID: "function_call",
			Content:   conversation.Content{conversation.ShellCallOutputBlock{}},
		},
	}

	markNativeShellResults(calls, results)

	native, ok := results[0].Content[0].(conversation.ShellCallOutputBlock)
	if !ok || native.OpenAINative == nil || !*native.OpenAINative {
		t.Fatalf("native result = %#v, want openAINative=true", results[0].Content)
	}
	function, ok := results[1].Content[0].(conversation.ShellCallOutputBlock)
	if !ok || function.OpenAINative == nil || *function.OpenAINative {
		t.Fatalf("function result = %#v, want openAINative=false", results[1].Content)
	}
}
