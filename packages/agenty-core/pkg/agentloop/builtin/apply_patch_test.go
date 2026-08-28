package builtin

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
)

func TestApplyPatchToolReturnsStructuredHelperResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses a POSIX script")
	}

	directory := t.TempDir()
	installApplyPatchFixture(t, directory, `#!/bin/sh
patch=$(cat)
case "$patch" in
  *"*** Begin Patch"*) ;;
  *) exit 2 ;;
esac
printf '%s\n' '{"success":true,"cwd":"/workspace","files":[{"path":"notes.txt","diff":"--- /dev/null\\n+++ b/notes.txt\\n@@ -0,0 +1 @@\\n+hello\\n","addedLines":1,"removedLines":0}]}'
`)
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))

	tool := &applyPatchTool{fileSystem: &fileSystem{}}
	input, err := json.Marshal(applyPatchArguments{Patch: "*** Begin Patch\n*** End Patch"})
	if err != nil {
		t.Fatal(err)
	}
	content, err := tool.Execute(t.Context(), agentloop.CallContext{Cwd: directory}, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) != 1 {
		t.Fatalf("content = %d blocks, want 1", len(content))
	}
	encoded, err := json.MarshalString(content[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(encoded, `\"addedLines\":1`) || !strings.Contains(encoded, `\"removedLines\":0`) {
		t.Errorf("tool result = %s", encoded)
	}
}

func TestApplyPatchToolReportsHelperFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses a POSIX script")
	}

	directory := t.TempDir()
	installApplyPatchFixture(t, directory, "#!/bin/sh\necho 'conflicting operations' >&2\nexit 1\n")
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))

	tool := &applyPatchTool{fileSystem: &fileSystem{}}
	input, err := json.Marshal(applyPatchArguments{Patch: "*** Begin Patch\n*** End Patch"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tool.Execute(t.Context(), agentloop.CallContext{Cwd: directory}, input)
	if err == nil || !strings.Contains(err.Error(), "conflicting operations") {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestApplyPatchToolLetsStartedHelperFinishAfterCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses a POSIX script")
	}

	directory := t.TempDir()
	started := filepath.Join(directory, "started")
	installApplyPatchFixture(t, directory, "#!/bin/sh\ntouch started\nsleep 0.2\nprintf '%s\\n' '{\"success\":true,\"cwd\":\"/workspace\",\"files\":[]}'\n")
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))

	tool := &applyPatchTool{fileSystem: &fileSystem{}}
	input, err := json.Marshal(applyPatchArguments{Patch: "*** Begin Patch\n*** End Patch"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, executeErr := tool.Execute(ctx, agentloop.CallContext{Cwd: directory}, input)
		result <- executeErr
	}()

	deadline := time.Now().Add(time.Second)
	for {
		if _, statErr := os.Stat(started); statErr == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("apply_patch helper did not start")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if executeErr := <-result; executeErr != nil {
		t.Fatalf("started helper returned error after cancellation: %v", executeErr)
	}
}

func TestApplyPatchToolRequiresPatch(t *testing.T) {
	tool := &applyPatchTool{fileSystem: &fileSystem{}}
	_, err := tool.Execute(context.Background(), agentloop.CallContext{}, []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "patch must not be empty") {
		t.Fatalf("Execute() error = %v", err)
	}
}

func installApplyPatchFixture(t *testing.T, directory, script string) {
	t.Helper()
	path := filepath.Join(directory, "apply_patch")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}
