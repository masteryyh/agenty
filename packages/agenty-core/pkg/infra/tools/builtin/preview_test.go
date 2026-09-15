package builtin

import (
	"strings"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
	"github.com/masteryyh/agenty-core/pkg/infra/tools"
)

func TestBuiltinApprovalPreviewsUseRegisteredSnapshot(t *testing.T) {
	registry := tools.NewRegistry()
	if err := RegisterAll(registry); err != nil {
		t.Fatal(err)
	}
	snapshot := registry.SnapshotToolRuntime().(tools.CallPreviewer)
	ctx := agentloop.CallContext{Cwd: t.TempDir()}
	for _, tt := range []struct{ name, input, title, detail string }{
		{"read_file", `{"path":"notes.txt","start_line":2,"end_line":5}`, "read this file: " + ctx.Cwd + "/notes.txt", "Start line: 2\nEnd line: 5"},
		{"ls", `{}`, "list this directory: " + ctx.Cwd, "immediate files"},
		{"glob", `{"pattern":"**/*.go"}`, "find files matching: **/*.go", ctx.Cwd},
		{"grep", `{"pattern":"TODO","glob":"*.go","case_sensitive":false}`, "search for: TODO", "Case sensitive: false"},
		{"shell", `{"commands":["echo first","echo second"],"stdin":"input","timeout_ms":3000}`, "run these commands:", "echo first\n\necho second\n\nStandard input:\ninput"},
		{"apply_patch", `{"patch":"*** Begin Patch\n*** Delete File: notes.txt\n*** End Patch"}`, "apply these changes:", "*** Delete File: notes.txt"},
		{"apply_patch", `{"operation":{"type":"update_file","path":"notes.txt","diff":"-old\n+new"}}`, "apply these changes:", "update_file: notes.txt\n\n-old\n+new"},
	} {
		t.Run(tt.name+tt.input, func(t *testing.T) {
			preview := snapshot.PreviewToolCall(ctx, conversation.ToolUseBlock{Name: tt.name, Input: []byte(tt.input)})
			if preview == nil || !strings.Contains(preview.Title, tt.title) || !strings.Contains(preview.Detail, tt.detail) {
				t.Fatalf("preview = %+v", preview)
			}
		})
	}
	registry.Unregister("read_file")
	if snapshot.PreviewToolCall(ctx, conversation.ToolUseBlock{Name: "read_file", Input: []byte(`{"path":"still-in-snapshot"}`)}) == nil {
		t.Fatal("snapshot lost preview after unregister")
	}
	if snapshot.PreviewToolCall(ctx, conversation.ToolUseBlock{Name: "mcp_lookup", Input: []byte(`{}`)}) != nil {
		t.Fatal("unknown tool received a builtin preview")
	}
	if snapshot.PreviewToolCall(ctx, conversation.ToolUseBlock{Name: "read_file", Input: []byte(`{"path":5}`)}) != nil {
		t.Fatal("invalid arguments should use generic preview")
	}
}
