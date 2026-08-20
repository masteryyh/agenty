package builtin_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGrep(t *testing.T) {
	t.Parallel()

	directory := createSearchFixture(t)
	encoded, err := executeTool(
		t,
		newRegistry(t),
		"grep",
		directory,
		`{"pattern":"needle","glob":"**/*.go","case_sensitive":false}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	result := decodeResult[struct {
		Matches []struct {
			Path   string `json:"path"`
			Line   int    `json:"line"`
			Column int    `json:"column"`
			Text   string `json:"text"`
		} `json:"matches"`
		Truncated bool `json:"truncated"`
	}](t, encoded)
	if result.Truncated || len(result.Matches) != 2 {
		t.Fatalf("grep result = %+v, want two matches", result)
	}
	if filepath.Base(result.Matches[0].Path) != "a.go" || result.Matches[0].Line != 2 || result.Matches[0].Column != 1 {
		t.Errorf("first match = %+v", result.Matches[0])
	}
	if filepath.Base(result.Matches[1].Path) != "b.go" || result.Matches[1].Line != 1 {
		t.Errorf("second match = %+v", result.Matches[1])
	}
}

func TestGrepValidatesPatternAndLimitsResults(t *testing.T) {
	t.Parallel()

	directory := createSearchFixture(t)
	registry := newRegistry(t)

	if _, err := executeTool(t, registry, "grep", directory, `{"pattern":"["}`); err == nil {
		t.Error("grep accepted an invalid regular expression")
	}
	if _, err := executeTool(
		t,
		registry,
		"grep",
		directory,
		`{"pattern":"needle","max_results":0}`,
	); err == nil {
		t.Error("grep accepted max_results zero")
	}

	encoded, err := executeTool(
		t,
		registry,
		"grep",
		directory,
		`{"pattern":"(?i)needle","glob":"**/*.go","max_results":1}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	result := decodeResult[struct {
		Matches   []any `json:"matches"`
		Truncated bool  `json:"truncated"`
	}](t, encoded)
	if len(result.Matches) != 1 || !result.Truncated {
		t.Errorf("limited grep result = %+v", result)
	}
}

func TestGlob(t *testing.T) {
	t.Parallel()

	directory := createSearchFixture(t)
	encoded, err := executeTool(
		t,
		newRegistry(t),
		"glob",
		directory,
		`{"pattern":"**/*.go"}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	result := decodeResult[struct {
		Root      string   `json:"root"`
		Paths     []string `json:"paths"`
		Truncated bool     `json:"truncated"`
	}](t, encoded)
	if result.Root != directory || result.Truncated {
		t.Errorf("glob metadata = %+v", result)
	}
	want := []string{
		filepath.Join(directory, "a.go"),
		filepath.Join(directory, "binary.go"),
		filepath.Join(directory, "nested", "b.go"),
	}
	if !slices.Equal(result.Paths, want) {
		t.Errorf("glob paths = %q, want %q", result.Paths, want)
	}
}

func TestListDirectory(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "b.txt"), []byte("bb"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "a-dir"), 0o755); err != nil {
		t.Fatal(err)
	}

	encoded, err := executeTool(t, newRegistry(t), "ls", directory, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	result := decodeResult[struct {
		Path    string `json:"path"`
		Entries []struct {
			Name string `json:"name"`
			Type string `json:"type"`
			Size int64  `json:"size"`
		} `json:"entries"`
		Truncated bool `json:"truncated"`
	}](t, encoded)
	if result.Path != directory || result.Truncated || len(result.Entries) != 2 {
		t.Fatalf("ls result = %+v", result)
	}
	if result.Entries[0].Name != "a-dir" || result.Entries[0].Type != "directory" {
		t.Errorf("first entry = %+v", result.Entries[0])
	}
	if result.Entries[1].Name != "b.txt" || result.Entries[1].Type != "file" || result.Entries[1].Size != 2 {
		t.Errorf("second entry = %+v", result.Entries[1])
	}
}

func createSearchFixture(t *testing.T) string {
	t.Helper()

	directory := t.TempDir()
	files := map[string][]byte{
		"a.go":         []byte("package a\nNeedle root\n"),
		"binary.go":    {0, 1, 2, 'n', 'e', 'e', 'd', 'l', 'e'},
		"nested/b.go":  []byte("needle nested\n"),
		"nested/c.txt": []byte("needle ignored\n"),
	}
	for relative, content := range files {
		path := filepath.Join(directory, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return strings.TrimSpace(directory)
}
