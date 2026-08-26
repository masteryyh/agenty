package builtin_test

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

func TestShellRunsCommandsInParallelAndPreservesOrder(t *testing.T) {
	t.Parallel()

	started := time.Now()
	output := executeShell(t, `{
		"commands":["sleep 0.4; printf first","sleep 0.4; printf second"],
		"timeout_ms":2000,
		"max_output_length":4096
	}`)
	if elapsed := time.Since(started); elapsed >= 700*time.Millisecond {
		t.Fatalf("shell commands took %v, want concurrent execution", elapsed)
	}
	if len(output.Output) != 2 {
		t.Fatalf("output length = %d, want 2", len(output.Output))
	}
	if output.Output[0].Stdout != "first" || output.Output[1].Stdout != "second" {
		t.Errorf("ordered stdout = %q, %q", output.Output[0].Stdout, output.Output[1].Stdout)
	}
	for index, result := range output.Output {
		if result.Outcome.Type != "exit" || result.Outcome.ExitCode == nil || *result.Outcome.ExitCode != 0 {
			t.Errorf("output %d outcome = %+v, want exit 0", index, result.Outcome)
		}
	}
}

func TestShellCapturesExitTimeoutAndOutputLimit(t *testing.T) {
	t.Parallel()

	started := time.Now()
	output := executeShell(t, `{
		"commands":["printf 12345; printf abcde >&2; exit 7","printf before; sleep 2 & wait"],
		"timeout_ms":30,
		"max_output_length":7
	}`)
	if elapsed := time.Since(started); elapsed >= 1500*time.Millisecond {
		t.Fatalf("timed out shell command took %v, want process tree termination", elapsed)
	}
	if output.MaxOutputLength != 7 {
		t.Errorf("max output length = %d, want 7", output.MaxOutputLength)
	}
	if output.Output[0].Stdout != "12345" || output.Output[0].Stderr != "ab" {
		t.Errorf("truncated output = %#v, want combined limit 7", output.Output[0])
	}
	if outcome := output.Output[0].Outcome; outcome.ExitCode == nil || *outcome.ExitCode != 7 {
		t.Errorf("exit outcome = %+v, want 7", outcome)
	}
	if output.Output[1].Stdout != "before" || output.Output[1].Outcome.Type != "timeout" {
		t.Errorf("timeout output = %+v", output.Output[1])
	}
}

func TestShellRegistryAttachesCallIDToSpecialOutput(t *testing.T) {
	t.Parallel()

	registry := newRegistry(t)
	results := registry.ExecuteBatch(t.Context(), agentloop.CallContext{}, []conversation.ToolUseBlock{
		{ID: "call_shell", Name: "shell", Input: []byte(`{"commands":["printf ok"]}`)},
	})
	if len(results) != 1 || len(results[0].Content) != 1 {
		t.Fatalf("results = %#v", results)
	}
	output, ok := results[0].Content[0].(conversation.ShellCallOutputBlock)
	if !ok || output.CallID != "call_shell" {
		t.Fatalf("special output = %#v, want call_shell", results[0].Content[0])
	}
}

func TestShellDefinitionDocumentsRuntimeAndCommandLimit(t *testing.T) {
	t.Parallel()

	tool, ok := newRegistry(t).Get("shell")
	if !ok {
		t.Fatal("shell is not registered")
	}
	definition := tool.Definition()
	if definition.Name != "shell" {
		t.Fatalf("shell definition = %q, want shell", definition.Name)
	}
	maxItems := definition.InputSchema.Properties["commands"].MaxItems
	const wantMaxItems = uint64(4)
	if maxItems == nil || *maxItems != wantMaxItems {
		t.Fatalf("commands maxItems = %v, want %d", maxItems, wantMaxItems)
	}
	for _, phrase := range []string{"independent, complete shell commands in parallel", "commands array contains separate commands", "same file", "zsh on macOS", "bash on Linux", "sh as a fallback"} {
		if !strings.Contains(definition.Description, phrase) {
			t.Errorf("description %q does not mention %q", definition.Description, phrase)
		}
	}
}

func TestShellRejectsInvalidArguments(t *testing.T) {
	t.Parallel()

	registry := newRegistry(t)
	tool, ok := registry.Get("shell")
	if !ok {
		t.Fatal("shell is not registered")
	}
	for _, input := range []string{
		`{"commands":[]}`,
		`{"commands":[" "]}`,
		`{"commands":["true"],"timeout_ms":0}`,
		`{"commands":["true"],"max_output_length":0}`,
		`{"commands":["true","true","true","true","true"]}`,
		`{"commands":["true","true"],"stdin":"patch"}`,
	} {
		if _, err := tool.Execute(context.Background(), agentloop.CallContext{}, []byte(input)); err == nil {
			t.Errorf("Execute(%s) succeeded", input)
		}
	}
}

func TestShellPassesStdinToSingleCommand(t *testing.T) {
	t.Parallel()

	command := "cat"
	if runtime.GOOS == "windows" {
		command = "more"
	}
	output := executeShell(t, fmt.Sprintf(`{"commands":[%q],"stdin":"patch input"}`, command))
	if output.Output[0].Stdout != "patch input" {
		t.Fatalf("stdin output = %q, want patch input", output.Output[0].Stdout)
	}
}

func TestShellReportsProcessStartErrors(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	if err := os.Remove(cwd); err != nil {
		t.Fatal(err)
	}
	output := executeShellWithCwd(t, cwd, `{"commands":["printf never-runs"]}`)
	if output.Output[0].Outcome.Type != "exit" || output.Output[0].Outcome.ExitCode == nil || *output.Output[0].Outcome.ExitCode != -1 {
		t.Fatalf("start error outcome = %+v, want exit -1", output.Output[0].Outcome)
	}
	if output.Output[0].Stderr == "" {
		t.Fatal("start error stderr is empty")
	}
}

func executeShell(t *testing.T, arguments string) conversation.ShellCallOutputBlock {
	t.Helper()
	return executeShellWithCwd(t, "", arguments)
}

func executeShellWithCwd(t *testing.T, cwd string, arguments string) conversation.ShellCallOutputBlock {
	t.Helper()

	registry := newRegistry(t)
	tool, ok := registry.Get("shell")
	if !ok {
		t.Fatal("shell is not registered")
	}
	content, err := tool.Execute(t.Context(), agentloop.CallContext{Cwd: cwd}, []byte(arguments))
	if err != nil {
		t.Fatal(err)
	}
	if len(content) != 1 {
		t.Fatalf("shell content length = %d, want 1", len(content))
	}
	output, ok := content[0].(conversation.ShellCallOutputBlock)
	if !ok {
		t.Fatalf("shell content = %T, want ShellCallOutputBlock", content[0])
	}
	return output
}
