package builtin

import (
	"os"
	"path/filepath"
	"testing"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/agentloop"
)

func TestReadOnlyToolsAutoApproveOnlyPathsInsideCwd(t *testing.T) {
	cwd := t.TempDir()
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "inside.txt"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "outside.txt"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}

	callContext := agentloop.CallContext{Cwd: cwd}
	tests := []struct {
		name    string
		check   func([]byte) bool
		inside  string
		outside string
	}{
		{
			name:    "read_file",
			check:   func(input []byte) bool { return (&readFileTool{}).CanAutoApprove(callContext, input) },
			inside:  `{"path":"inside.txt"}`,
			outside: `{"path":"` + filepath.Join(external, "outside.txt") + `"}`,
		},
		{
			name:    "grep",
			check:   func(input []byte) bool { return (&grepTool{}).CanAutoApprove(callContext, input) },
			inside:  `{"pattern":"inside","path":"."}`,
			outside: `{"pattern":"outside","path":"` + external + `"}`,
		},
		{
			name:    "glob",
			check:   func(input []byte) bool { return (&globTool{}).CanAutoApprove(callContext, input) },
			inside:  `{"pattern":"**","path":"."}`,
			outside: `{"pattern":"**","path":"` + external + `"}`,
		},
		{
			name:    "ls",
			check:   func(input []byte) bool { return (&listTool{}).CanAutoApprove(callContext, input) },
			inside:  `{"path":"."}`,
			outside: `{"path":"` + external + `"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !test.check([]byte(test.inside)) {
				t.Fatalf("inside cwd input was not approved: %s", test.inside)
			}
			if test.check([]byte(test.outside)) {
				t.Fatalf("outside cwd input was approved: %s", test.outside)
			}
		})
	}

	if err := os.Symlink(external, filepath.Join(cwd, "escaped")); err != nil {
		t.Fatal(err)
	}
	if (&readFileTool{}).CanAutoApprove(callContext, []byte(`{"path":"escaped/outside.txt"}`)) {
		t.Fatal("symlink escape was approved")
	}
}

func TestApplyPatchAutoApprovesOnlyCwdPaths(t *testing.T) {
	cwd := t.TempDir()
	external := t.TempDir()
	tool := &applyPatchTool{}
	callContext := agentloop.CallContext{Cwd: cwd}

	tests := []struct {
		name string
		args applyPatchArguments
		want bool
	}{
		{
			name: "custom patch inside cwd",
			args: applyPatchArguments{Patch: "*** Begin Patch\n*** Update File: notes.txt\n*** Move to: archived/notes.txt\n@@\n-old\n+new\n*** End Patch"},
			want: true,
		},
		{
			name: "custom patch traverses outside cwd",
			args: applyPatchArguments{Patch: "*** Begin Patch\n*** Add File: ../outside.txt\n+outside\n*** End Patch"},
			want: false,
		},
		{
			name: "native operation inside cwd",
			args: applyPatchArguments{Operation: &conversation.ApplyPatchOperation{
				Type: conversation.ApplyPatchCreateFile,
				Path: "notes.txt",
			}},
			want: true,
		},
		{
			name: "native operation moves outside cwd",
			args: applyPatchArguments{Operation: &conversation.ApplyPatchOperation{
				Type:   conversation.ApplyPatchUpdateFile,
				Path:   "notes.txt",
				MoveTo: filepath.Join(external, "notes.txt"),
			}},
			want: false,
		},
		{
			name: "invalid native operation",
			args: applyPatchArguments{Operation: &conversation.ApplyPatchOperation{
				Type: "replace_everything",
				Path: "notes.txt",
			}},
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input, err := json.Marshal(test.args)
			if err != nil {
				t.Fatal(err)
			}
			if got := tool.CanAutoApprove(callContext, input); got != test.want {
				t.Fatalf("CanAutoApprove() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestSensitiveTargetsRequireReview(t *testing.T) {
	cwd := t.TempDir()
	callContext := agentloop.CallContext{Cwd: cwd}
	for _, test := range []struct {
		path  string
		allow bool
	}{
		{"src/main.go", true}, {".env.example", true}, {".env.local.template", true},
		{".env", false}, {".env.production", false}, {"config/.env", false},
		{".ssh/id_ed25519", false}, {".aws/credentials", false}, {".git/config", false},
		{".npmrc", false}, {".docker/config.json", false}, {"keys/client.pem", false},
	} {
		t.Run(test.path, func(t *testing.T) {
			target := filepath.Join(cwd, test.path)
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte("fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
			input, _ := json.Marshal(readFileArguments{Path: test.path})
			if got := (&readFileTool{}).CanAutoApprove(callContext, input); got != test.allow {
				t.Fatalf("read allowed = %v", got)
			}
			patchInput, _ := json.Marshal(applyPatchArguments{Operation: &conversation.ApplyPatchOperation{Type: conversation.ApplyPatchDeleteFile, Path: test.path}})
			if got := (&applyPatchTool{}).CanAutoApprove(callContext, patchInput); got != test.allow {
				t.Fatalf("delete allowed = %v", got)
			}
		})
	}
	if err := os.Symlink(filepath.Join(cwd, ".env"), filepath.Join(cwd, "ordinary.txt")); err != nil {
		t.Fatal(err)
	}
	if (&readFileTool{}).CanAutoApprove(callContext, []byte(`{"path":"ordinary.txt"}`)) {
		t.Fatal("sensitive symlink target was allowed")
	}
	input, _ := json.Marshal(applyPatchArguments{Patch: "*** Begin Patch\n*** Update File: src/main.go\n*** Move to: .env\n@@\n-old\n+new\n*** End Patch"})
	if (&applyPatchTool{}).CanAutoApprove(callContext, input) {
		t.Fatal("sensitive move target was allowed")
	}
	if !(&listTool{}).CanAutoApprove(callContext, []byte(`{"path":".ssh"}`)) {
		t.Fatal("listing filenames should remain allowed")
	}
}

func TestGrepChecksHiddenTargetsWithoutReadingContents(t *testing.T) {
	cwd := t.TempDir()
	for _, name := range []string{"main.go", ".env", ".env.example"} {
		if err := os.WriteFile(filepath.Join(cwd, name), []byte("test fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		input string
		allow bool
	}{
		{`{"pattern":"token","path":"."}`, false},
		{`{"pattern":"token","path":".","glob":"*.go"}`, true},
		{`{"pattern":"token","path":".","glob":".env"}`, false},
		{`{"pattern":"token","path":".","glob":".env.example"}`, true},
		{`{"pattern":"token","path":".env"}`, false},
		{`{"pattern":"token","path":"main.go"}`, true},
	} {
		if got := (&grepTool{}).CanAutoApprove(agentloop.CallContext{Cwd: cwd}, []byte(test.input)); got != test.allow {
			t.Errorf("%s allowed = %v", test.input, got)
		}
	}
	if !sensitiveApprovalPath("/etc/hosts") || !sensitiveApprovalPath("/private/etc/hosts") || !sensitiveApprovalPath("/System/Library/config") {
		t.Fatal("OS paths not detected")
	}
}
