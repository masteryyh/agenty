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

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty/pkg/config"
)

func TestReadFileTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tool := &ReadFileTool{cfg: &config.AppConfig{AllowedPaths: []string{dir}}}
	args, _ := json.MarshalString(map[string]string{"path": path})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", result)
	}
}

func TestReadFileToolNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.txt")

	tool := &ReadFileTool{cfg: &config.AppConfig{AllowedPaths: []string{dir}}}
	args, _ := json.MarshalString(map[string]string{"path": path})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestReadFileToolDenied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tool := &ReadFileTool{cfg: &config.AppConfig{AllowedPaths: []string{t.TempDir()}}}
	args, _ := json.MarshalString(map[string]string{"path": path})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for denied path")
	}
}

func TestWriteFileTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "output.txt")

	tool := &WriteFileTool{cfg: &config.AppConfig{AllowedPaths: []string{dir}}}
	args, _ := json.MarshalString(map[string]string{"path": path, "content": "test content"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "successfully wrote") {
		t.Fatalf("expected success message, got '%s'", result)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(data) != "test content" {
		t.Fatalf("expected 'test content', got '%s'", string(data))
	}
}

func TestWriteFileToolDenied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")

	tool := &WriteFileTool{cfg: &config.AppConfig{AllowedPaths: []string{t.TempDir()}}}
	args, _ := json.MarshalString(map[string]string{"path": path, "content": "test content"})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for denied path")
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

	tool := &ListDirectoryTool{cfg: &config.AppConfig{AllowedPaths: []string{dir}}}
	args, _ := json.MarshalString(map[string]string{"path": dir})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "[file] file1.txt") {
		t.Fatalf("expected file entry, got '%s'", result)
	}
	if !strings.Contains(result, "[dir] subdir") {
		t.Fatalf("expected dir entry, got '%s'", result)
	}
}

func TestListDirectoryToolDenied(t *testing.T) {
	dir := t.TempDir()

	tool := &ListDirectoryTool{cfg: &config.AppConfig{AllowedPaths: []string{t.TempDir()}}}
	args, _ := json.MarshalString(map[string]string{"path": dir})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for denied path")
	}
}

func TestToolDefinitions(t *testing.T) {
	cfg := &config.AppConfig{AllowedPaths: []string{t.TempDir()}}
	tool1 := &ReadFileTool{cfg: cfg}
	tool2 := &WriteFileTool{cfg: cfg}
	tool3 := &ListDirectoryTool{cfg: cfg}

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
