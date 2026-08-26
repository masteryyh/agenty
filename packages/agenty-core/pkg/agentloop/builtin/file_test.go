package builtin_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	json "github.com/bytedance/sonic"
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

func TestReadFileAbsolutePaths(t *testing.T) {
	directory := t.TempDir()

	absolutePath := filepath.Join(t.TempDir(), "absolute.txt")
	if err := os.WriteFile(absolutePath, []byte("absolute"), 0o644); err != nil {
		t.Fatal(err)
	}

	homeDirectory := t.TempDir()
	homeVariable := "HOME"
	if runtime.GOOS == "windows" {
		homeVariable = "USERPROFILE"
	}
	t.Setenv(homeVariable, homeDirectory)

	homePath := filepath.Join(homeDirectory, "from-home.txt")
	if err := os.WriteFile(homePath, []byte("home"), 0o644); err != nil {
		t.Fatal(err)
	}

	environmentDirectory := t.TempDir()
	t.Setenv("AGENTY_READ_FILE_ROOT", environmentDirectory)
	environmentPath := filepath.Join(environmentDirectory, "from-environment.txt")
	if err := os.WriteFile(environmentPath, []byte("environment"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		path         string
		wantPath     string
		wantContents string
	}{
		{
			name:         "native absolute path",
			path:         absolutePath,
			wantPath:     absolutePath,
			wantContents: "absolute",
		},
		{
			name:         "home shorthand",
			path:         "~/from-home.txt",
			wantPath:     homePath,
			wantContents: "home",
		},
		{
			name:         "braced POSIX environment variable",
			path:         "${AGENTY_READ_FILE_ROOT}/from-environment.txt",
			wantPath:     environmentPath,
			wantContents: "environment",
		},
		{
			name:         "POSIX environment variable",
			path:         "$AGENTY_READ_FILE_ROOT/from-environment.txt",
			wantPath:     environmentPath,
			wantContents: "environment",
		},
		{
			name:         "Windows environment variable",
			path:         "%AGENTY_READ_FILE_ROOT%/from-environment.txt",
			wantPath:     environmentPath,
			wantContents: "environment",
		},
		{
			name:         "PowerShell environment variable",
			path:         "$env:AGENTY_READ_FILE_ROOT/from-environment.txt",
			wantPath:     environmentPath,
			wantContents: "environment",
		},
	}

	registry := newRegistry(t)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encodedPath, err := json.MarshalString(test.path)
			if err != nil {
				t.Fatal(err)
			}

			encoded, err := executeTool(t, registry, "read_file", directory, `{"path":`+encodedPath+`}`)
			if err != nil {
				t.Fatal(err)
			}

			result := decodeResult[struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}](t, encoded)
			if result.Path != test.wantPath {
				t.Errorf("path = %q, want %q", result.Path, test.wantPath)
			}
			if result.Content != "1: "+test.wantContents {
				t.Errorf("content = %q, want %q", result.Content, "1: "+test.wantContents)
			}
		})
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
		{name: "explicit zero start", arguments: `{"path":"short.txt","start_line":0}`, wantError: "start_line must be positive"},
		{name: "reversed range", arguments: `{"path":"short.txt","start_line":2,"end_line":1}`, wantError: "start_line must not exceed end_line"},
		{name: "start after end of file", arguments: `{"path":"short.txt","start_line":3}`, wantError: "exceeds file length 2"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := executeTool(t, registry, "read_file", directory, test.arguments)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want containing %q", err, test.wantError)
			}
		})
	}
}
