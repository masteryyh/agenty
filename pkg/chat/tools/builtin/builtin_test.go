package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0o644)

	tool := &ReadFileTool{}
	args, _ := json.Marshal(map[string]string{"path": path})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", result)
	}
}

func TestReadFileToolNotFound(t *testing.T) {
	tool := &ReadFileTool{}
	args, _ := json.Marshal(map[string]string{"path": "/nonexistent/path/file.txt"})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestWriteFileTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "output.txt")

	tool := &WriteFileTool{}
	args, _ := json.Marshal(map[string]string{"path": path, "content": "test content"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(data) != "test content" {
		t.Fatalf("expected 'test content', got '%s'", string(data))
	}
}

func TestListDirectoryTool(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("a"), 0o644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0o755)

	tool := &ListDirectoryTool{}
	args, _ := json.Marshal(map[string]string{"path": dir})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestRegisterAll(t *testing.T) {
	tool1 := &ReadFileTool{}
	tool2 := &WriteFileTool{}
	tool3 := &ListDirectoryTool{}

	if tool1.Definition().Name != "read_file" {
		t.Fatalf("expected 'read_file', got '%s'", tool1.Definition().Name)
	}
	if tool2.Definition().Name != "write_file" {
		t.Fatalf("expected 'write_file', got '%s'", tool2.Definition().Name)
	}
	if tool3.Definition().Name != "list_directory" {
		t.Fatalf("expected 'list_directory', got '%s'", tool3.Definition().Name)
	}
}
