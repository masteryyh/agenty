/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package builtin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	json "github.com/bytedance/sonic"
	"github.com/gofrs/flock"
	"github.com/masteryyh/agenty/pkg/tools"
)

func TestReadFileTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tool := &ReadFileTool{}
	args, _ := json.MarshalString(map[string]string{"path": path})
	result, err := tool.Execute(context.Background(), tools.ToolCallContext{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	if decoded.Content != "1: hello world" {
		t.Fatalf("expected '1: hello world', got '%s'", decoded.Content)
	}
}

func TestReadFileToolNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.txt")

	tool := &ReadFileTool{}
	args, _ := json.MarshalString(map[string]string{"path": path})
	_, err := tool.Execute(context.Background(), tools.ToolCallContext{}, args)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestWriteFileTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "output.txt")

	tool := &WriteFileTool{}
	args, _ := json.MarshalString(map[string]string{"path": path, "content": "test content"})
	result, err := tool.Execute(context.Background(), tools.ToolCallContext{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded struct {
		Path         string `json:"path"`
		BytesWritten int    `json:"bytesWritten"`
	}
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	if decoded.Path != path || decoded.BytesWritten != len("test content") {
		t.Fatalf("unexpected result: %#v", decoded)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(data) != "test content" {
		t.Fatalf("expected 'test content', got '%s'", string(data))
	}
}

func TestWriteFileToolWaitsForFileLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "locked.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	existingLock := flock.New(fileLockPath(path))
	if err := existingLock.Lock(); err != nil {
		t.Fatalf("failed to lock test file: %v", err)
	}
	defer func() { _ = existingLock.Unlock() }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	tool := &WriteFileTool{}
	args, _ := json.MarshalString(map[string]string{"path": path, "content": "new"})
	_, err := tool.Execute(ctx, tools.ToolCallContext{}, args)
	if err == nil {
		t.Fatal("expected error while file lock is held")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected context deadline error, got %v", err)
	}
}

func TestReleaseFileLockRemovesLockFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "locked.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	fileLock, err := acquireFileLock(context.Background(), path, true)
	if err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}
	lockPath := fileLockPath(path)
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("expected lock file to exist: %v", err)
	}

	if err := releaseFileLock(fileLock); err != nil {
		t.Fatalf("failed to release lock: %v", err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("expected lock file to be removed, got %v", err)
	}
}

func TestReleaseFileLockKeepsLockFileWhenSharedLockRemains(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shared.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	firstLock, err := acquireFileLock(context.Background(), path, false)
	if err != nil {
		t.Fatalf("failed to acquire first lock: %v", err)
	}
	secondLock, err := acquireFileLock(context.Background(), path, false)
	if err != nil {
		_ = releaseFileLock(firstLock)
		t.Fatalf("failed to acquire second lock: %v", err)
	}

	lockPath := fileLockPath(path)
	if err := releaseFileLock(firstLock); err != nil {
		_ = releaseFileLock(secondLock)
		t.Fatalf("failed to release first lock: %v", err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		_ = releaseFileLock(secondLock)
		t.Fatalf("expected lock file to remain: %v", err)
	}

	if err := releaseFileLock(secondLock); err != nil {
		t.Fatalf("failed to release second lock: %v", err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("expected lock file to be removed, got %v", err)
	}
}

func TestValidateFileToolPathRejectsRestrictedPaths(t *testing.T) {
	cases := []string{
		"/dev/zero",
		"/etc/passwd",
		`C:\Windows\System32\drivers\etc\hosts`,
	}
	for _, path := range cases {
		if err := validatePath(path); err == nil {
			t.Fatalf("expected restricted path error for %s", path)
		}
	}

	if err := validatePath("/etc2/passwd"); err != nil {
		t.Fatalf("unexpected error for non-sensitive prefix: %v", err)
	}
}

func TestWriteFileToolRejectsSensitiveSymlinkParent(t *testing.T) {
	if _, err := os.Stat("/etc"); err != nil {
		t.Skip("/etc is not available on this platform")
	}

	dir := t.TempDir()
	linkPath := filepath.Join(dir, "system")
	if err := os.Symlink("/etc", linkPath); err != nil {
		t.Skipf("failed to create symlink: %v", err)
	}

	tool := &WriteFileTool{}
	args, _ := json.MarshalString(map[string]string{"path": filepath.Join(linkPath, "agenty-test-file"), "content": "x"})
	_, err := tool.Execute(context.Background(), tools.ToolCallContext{}, args)
	if err == nil {
		t.Fatal("expected error for sensitive symlink target")
	}
	if !strings.Contains(err.Error(), "run_shell_command") {
		t.Fatalf("expected command guidance, got %v", err)
	}
}

func TestListDirectoryTool(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	tool := &ListDirectoryTool{}
	args, _ := json.MarshalString(map[string]string{"path": dir})
	result, err := tool.Execute(context.Background(), tools.ToolCallContext{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded struct {
		Entries []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	entries := map[string]string{}
	for _, entry := range decoded.Entries {
		entries[entry.Name] = entry.Type
	}
	if entries["file1.txt"] != "file" {
		t.Fatalf("expected file entry, got %#v", decoded.Entries)
	}
	if entries["subdir"] != "dir" {
		t.Fatalf("expected dir entry, got %#v", decoded.Entries)
	}
}

func TestListDirectoryToolRejectsSensitivePath(t *testing.T) {
	if _, err := os.Stat("/etc"); err != nil {
		t.Skip("/etc is not available on this platform")
	}

	tool := &ListDirectoryTool{}
	args, _ := json.MarshalString(map[string]string{"path": "/etc"})
	_, err := tool.Execute(context.Background(), tools.ToolCallContext{}, args)
	if err == nil {
		t.Fatal("expected error for sensitive system path")
	}
	if !strings.Contains(err.Error(), "run_shell_command") {
		t.Fatalf("expected command guidance, got %v", err)
	}
}

func TestReadFileToolLineRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lines.txt")
	content := "line1\nline2\nline3\nline4\nline5"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tool := &ReadFileTool{}

	args, _ := json.Marshal(map[string]any{"path": path, "startLine": 2, "endLine": 4})
	result, err := tool.Execute(context.Background(), tools.ToolCallContext{}, string(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	if decoded.Content != "2: line2\n3: line3\n4: line4" {
		t.Fatalf("expected '2: line2\\n3: line3\\n4: line4', got '%s'", decoded.Content)
	}

	args, _ = json.Marshal(map[string]any{"path": path, "startLine": 10, "endLine": 12})
	_, err = tool.Execute(context.Background(), tools.ToolCallContext{}, string(args))
	if err == nil {
		t.Fatal("expected error for out-of-range startLine")
	}

	args, _ = json.Marshal(map[string]any{"path": path, "startLine": 3, "endLine": 2})
	_, err = tool.Execute(context.Background(), tools.ToolCallContext{}, string(args))
	if err == nil {
		t.Fatal("expected error for startLine > endLine")
	}
}

func TestReadFileToolLineNumbers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lines.txt")
	content := "line1\nline2\nline3"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tool := &ReadFileTool{}

	args, _ := json.Marshal(map[string]any{"path": path, "startLine": 2, "endLine": 3})
	result, err := tool.Execute(context.Background(), tools.ToolCallContext{}, string(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	if decoded.Content != "2: line2\n3: line3" {
		t.Fatalf("expected numbered lines, got '%s'", decoded.Content)
	}
}

func TestReplaceInFileTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replace.txt")
	content := "line1\nline2\nline3\nline4\nline5"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tool := &ReplaceInFileTool{}
	args, _ := json.Marshal(map[string]any{
		"path":       path,
		"startLine":  2,
		"endLine":    3,
		"newContent": "replaced2\nreplaced3",
	})
	result, err := tool.Execute(context.Background(), tools.ToolCallContext{}, string(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var decoded struct {
		Path              string `json:"path"`
		StartLine         int    `json:"startLine"`
		EndLine           int    `json:"endLine"`
		ReplacedLineCount int    `json:"replacedLineCount"`
	}
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	if decoded.Path == "" || decoded.StartLine != 2 || decoded.EndLine != 3 || decoded.ReplacedLineCount != 2 {
		t.Fatalf("unexpected result: %#v", decoded)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected := "line1\nreplaced2\nreplaced3\nline4\nline5"
	if string(data) != expected {
		t.Fatalf("expected '%s', got '%s'", expected, string(data))
	}
}

func TestReplaceInFileToolErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "err.txt")
	if err := os.WriteFile(path, []byte("a\nb\nc"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tool := &ReplaceInFileTool{}

	args, _ := json.Marshal(map[string]any{"path": path, "startLine": 0, "endLine": 1, "newContent": "x"})
	_, err := tool.Execute(context.Background(), tools.ToolCallContext{}, string(args))
	if err == nil {
		t.Fatal("expected error for startLine < 1")
	}

	args, _ = json.Marshal(map[string]any{"path": path, "startLine": 2, "endLine": 1, "newContent": "x"})
	_, err = tool.Execute(context.Background(), tools.ToolCallContext{}, string(args))
	if err == nil {
		t.Fatal("expected error for endLine < startLine")
	}

	args, _ = json.Marshal(map[string]any{"path": filepath.Join(dir, "missing.txt"), "startLine": 1, "endLine": 1, "newContent": "x"})
	_, err = tool.Execute(context.Background(), tools.ToolCallContext{}, string(args))
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
