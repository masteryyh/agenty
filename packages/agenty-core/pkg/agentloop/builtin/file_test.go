package builtin_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFile(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "nested", "example.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("alpha\nbeta\n世界\ndelta\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	encoded, err := executeTool(
		t,
		newRegistry(t),
		"read_file",
		directory,
		`{"path":"nested/example.txt","start_line":2,"end_line":3}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	result := decodeResult[struct {
		Path      string `json:"path"`
		Content   string `json:"content"`
		StartLine int    `json:"startLine"`
		EndLine   int    `json:"endLine"`
		Truncated bool   `json:"truncated"`
	}](t, encoded)
	if result.Path != path {
		t.Errorf("path = %q, want %q", result.Path, path)
	}
	if result.Content != "2: beta\n3: 世界" {
		t.Errorf("content = %q", result.Content)
	}
	if result.StartLine != 2 || result.EndLine != 3 || result.Truncated {
		t.Errorf("range result = %+v", result)
	}
}

func TestReadFileValidatesLineBounds(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "short.txt"), []byte("one\ntwo"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := newRegistry(t)

	tests := []struct {
		name      string
		arguments string
		wantError string
	}{
		{
			name:      "explicit zero start",
			arguments: `{"path":"short.txt","start_line":0}`,
			wantError: "start_line must be positive",
		},
		{
			name:      "reversed range",
			arguments: `{"path":"short.txt","start_line":2,"end_line":1}`,
			wantError: "start_line must not exceed end_line",
		},
		{
			name:      "start after end of file",
			arguments: `{"path":"short.txt","start_line":3}`,
			wantError: "exceeds file length 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := executeTool(t, registry, "read_file", directory, tt.arguments)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantError)
			}
		})
	}
}

func TestWriteFileRequiresContentField(t *testing.T) {
	t.Parallel()

	_, err := executeTool(t, newRegistry(t), "write_file", t.TempDir(), `{"path":"empty.txt"}`)
	if err == nil || !strings.Contains(err.Error(), "content is required") {
		t.Fatalf("error = %v, want missing content error", err)
	}
}

func TestWriteFileCreatesAndOverwritesFile(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	registry := newRegistry(t)
	path := filepath.Join(directory, "nested", "created.txt")

	encoded, err := executeTool(
		t,
		registry,
		"write_file",
		directory,
		`{"path":"nested/created.txt","content":"first"}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	created := decodeResult[struct {
		Path         string `json:"path"`
		BytesWritten int    `json:"bytesWritten"`
		Created      bool   `json:"created"`
	}](t, encoded)
	if !created.Created || created.Path != path || created.BytesWritten != 5 {
		t.Errorf("created result = %+v", created)
	}

	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	encoded, err = executeTool(
		t,
		registry,
		"write_file",
		directory,
		`{"path":"nested/created.txt","content":"second"}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	overwritten := decodeResult[struct {
		BytesWritten int  `json:"bytesWritten"`
		Created      bool `json:"created"`
	}](t, encoded)
	if overwritten.Created || overwritten.BytesWritten != 6 {
		t.Errorf("overwrite result = %+v", overwritten)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "second" {
		t.Errorf("file content = %q, want second", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %o, want 600", info.Mode().Perm())
	}
}

func TestPatchFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		original    string
		arguments   string
		wantContent string
		wantCount   int
		wantError   string
	}{
		{
			name:        "unique replacement",
			original:    "before old after",
			arguments:   `{"path":"file.txt","old_text":"old","new_text":"new"}`,
			wantContent: "before new after",
			wantCount:   1,
		},
		{
			name:        "replace all",
			original:    "old and old",
			arguments:   `{"path":"file.txt","old_text":"old","new_text":"new","replace_all":true}`,
			wantContent: "new and new",
			wantCount:   2,
		},
		{
			name:      "reject missing new text",
			original:  "old",
			arguments: `{"path":"file.txt","old_text":"old"}`,
			wantError: "new_text is required",
		},
		{
			name:      "reject ambiguous replacement",
			original:  "old and old",
			arguments: `{"path":"file.txt","old_text":"old","new_text":"new"}`,
			wantError: "old_text occurs 2 times",
		},
		{
			name:      "reject missing text",
			original:  "unchanged",
			arguments: `{"path":"file.txt","old_text":"missing","new_text":"new"}`,
			wantError: "old_text was not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			directory := t.TempDir()
			path := filepath.Join(directory, "file.txt")
			if err := os.WriteFile(path, []byte(tt.original), 0o640); err != nil {
				t.Fatal(err)
			}

			encoded, err := executeTool(t, newRegistry(t), "patch_file", directory, tt.arguments)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			result := decodeResult[struct {
				Replacements int `json:"replacements"`
				BytesWritten int `json:"bytesWritten"`
			}](t, encoded)
			if result.Replacements != tt.wantCount || result.BytesWritten != len(tt.wantContent) {
				t.Errorf("patch result = %+v", result)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tt.wantContent {
				t.Errorf("file content = %q, want %q", data, tt.wantContent)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o640 {
				t.Errorf("file mode = %o, want 640", info.Mode().Perm())
			}
		})
	}
}

func TestDeleteFile(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	filePath := filepath.Join(directory, "remove.txt")
	if err := os.WriteFile(filePath, []byte("remove me"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := newRegistry(t)

	encoded, err := executeTool(
		t,
		registry,
		"delete_file",
		directory,
		`{"path":"remove.txt"}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	result := decodeResult[struct {
		Path    string `json:"path"`
		Deleted bool   `json:"deleted"`
	}](t, encoded)
	if !result.Deleted || result.Path != filePath {
		t.Errorf("delete result = %+v", result)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("deleted file stat error = %v, want not exist", err)
	}

	_, err = executeTool(
		t,
		registry,
		"delete_file",
		directory,
		fmt.Sprintf(`{"path":%q}`, directory),
	)
	if err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("directory delete error = %v", err)
	}
	if _, err := os.Stat(directory); err != nil {
		t.Fatalf("directory was removed: %v", err)
	}
}
