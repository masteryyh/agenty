package builtin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

func TestParsePatchEnvelopePreservesRepeatedFileOperations(t *testing.T) {
	t.Parallel()

	patch := `*** Begin Patch
*** Update File: notes.txt
@@
-one
+two
*** Update File: notes.txt
@@
-two
+three
*** End Patch`
	operations, err := parsePatchEnvelope(patch)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 2 {
		t.Fatalf("operations = %d, want 2", len(operations))
	}
	for index, operation := range operations {
		if operation.Path != "notes.txt" {
			t.Errorf("operation %d path = %q, want notes.txt", index, operation.Path)
		}
	}
	if operations[0].Diff == operations[1].Diff {
		t.Errorf("repeated operations were collapsed: %#v", operations)
	}
}

func TestApplyPatchToolExecutesOperationsInOrder(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	tool := &applyPatchTool{fileSystem: &fileSystem{}}
	patch := `*** Begin Patch
*** Add File: notes.txt
+one
*** Update File: notes.txt
@@
-one
+two
*** Update File: notes.txt
@@
-two
+three
*** End Patch`
	content, err := executeApplyPatchTool(t, tool, directory, applyPatchArguments{Patch: patch})
	if err != nil {
		t.Fatal(err)
	}
	if len(content) != 1 {
		t.Fatalf("content = %d blocks, want 1", len(content))
	}
	data, err := os.ReadFile(filepath.Join(directory, "notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "three" {
		t.Errorf("notes.txt = %q, want three", data)
	}
}

func TestApplyPatchToolSupportsMoveThenUpdateDestination(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "old.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &applyPatchTool{fileSystem: &fileSystem{}}
	patch := `*** Begin Patch
*** Update File: old.txt
*** Move to: new.txt
@@
-one
+two
*** Update File: new.txt
@@
-two
+three
*** End Patch`
	if _, err := executeApplyPatchTool(t, tool, directory, applyPatchArguments{Patch: patch}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("old.txt still exists: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "new.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "three\n" {
		t.Errorf("new.txt = %q, want %q", data, "three\n")
	}
}

func TestApplyPatchToolKeepsEarlierOperationsOnFailure(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	tool := &applyPatchTool{fileSystem: &fileSystem{}}
	patch := `*** Begin Patch
*** Add File: created.txt
+created
*** Update File: missing.txt
@@
-missing
+updated
*** End Patch`
	_, err := executeApplyPatchTool(t, tool, directory, applyPatchArguments{Patch: patch})
	if err == nil || !strings.Contains(err.Error(), "operation 2") {
		t.Fatalf("Execute() error = %v, want operation 2 failure", err)
	}
	data, readErr := os.ReadFile(filepath.Join(directory, "created.txt"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "created" {
		t.Errorf("created.txt = %q, want created", data)
	}
}

func TestApplyPatchToolExecutesNativeOperation(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	tool := &applyPatchTool{fileSystem: &fileSystem{}}
	operation := conversation.ApplyPatchOperation{
		Type: conversation.ApplyPatchCreateFile,
		Path: "native.txt",
		Diff: "+native",
	}
	if _, err := executeApplyPatchTool(t, tool, directory, applyPatchArguments{Operation: &operation}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "native.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "native" {
		t.Errorf("native.txt = %q, want native", data)
	}
}

func TestApplyPatchToolRejectsMalformedEnvelopeBeforeWriting(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	tool := &applyPatchTool{fileSystem: &fileSystem{}}
	patch := `*** Begin Patch
*** Add File: created.txt
+created`
	_, err := executeApplyPatchTool(t, tool, directory, applyPatchArguments{Patch: patch})
	if err == nil || !strings.Contains(err.Error(), "must end") {
		t.Fatalf("Execute() error = %v, want missing end marker", err)
	}
	if _, statErr := os.Stat(filepath.Join(directory, "created.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("created.txt exists after parse failure: %v", statErr)
	}
}

func executeApplyPatchTool(
	t *testing.T,
	tool *applyPatchTool,
	cwd string,
	arguments applyPatchArguments,
) (conversation.Content, error) {
	t.Helper()

	input, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	return tool.Execute(t.Context(), agentloop.CallContext{Cwd: cwd}, input)
}
