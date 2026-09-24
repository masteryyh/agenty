package builtin

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
)

func TestTextEditorToolUsesFileeditTextEditorMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses a POSIX script")
	}

	directory := t.TempDir()
	installTextEditorFixture(t, directory, `#!/bin/sh
[ "$1" = "text_editor" ] || { echo "expected text_editor mode" >&2; exit 2; }
input=$(cat)
case "$input" in
  *'"command":"str_replace"'*) ;;
  *) echo "missing str_replace request" >&2; exit 2 ;;
esac
printf '%s\n' '{"success":true,"cwd":"/workspace","files":[{"path":"notes.txt","diff":"--- a/notes.txt\n+++ b/notes.txt\n@@\n-old\n+new\n","addedLines":1,"removedLines":1}]}'
`)
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))

	input, err := json.Marshal(textEditorArguments{
		Command: "str_replace",
		Path:    "notes.txt",
		OldStr:  new("old"),
		NewStr:  new("new"),
	})
	if err != nil {
		t.Fatal(err)
	}
	tool := &textEditorTool{fileSystem: &fileSystem{}}
	content, err := tool.Execute(t.Context(), agentloop.CallContext{Cwd: directory}, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) != 1 {
		t.Fatalf("content = %d blocks, want 1", len(content))
	}
}

func TestTextEditorToolViewsRequestedRange(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "notes.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(textEditorArguments{
		Command:   "view",
		Path:      "notes.txt",
		ViewRange: []int{2, 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	tool := &textEditorTool{fileSystem: &fileSystem{}}
	content, err := tool.Execute(t.Context(), agentloop.CallContext{Cwd: directory}, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) != 1 {
		t.Fatalf("content = %d blocks, want 1", len(content))
	}
	text, ok := content[0].(conversation.TextBlock)
	if !ok || strings.TrimSpace(text.Text) != "2: two\n3: three" {
		t.Fatalf("view content = %#v", content)
	}
}

func installTextEditorFixture(t *testing.T, directory, script string) {
	t.Helper()
	path := filepath.Join(directory, "fileedit")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}
