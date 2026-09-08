package skill

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, root, directory, name, description string) string {
	t.Helper()
	path := filepath.Join(root, directory)
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	location := filepath.Join(path, "SKILL.md")
	content := "---\nname: " + name + "\ndescription: " + strconv.Quote(description) + "\n---\n\n# " + name + "\n"
	if err := os.WriteFile(location, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return location
}

func TestScanRootsUsesDirectoryPrecedenceAndWarnsOnNameMismatch(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	firstLocation := writeSkill(t, first, "same", "same", "first")
	writeSkill(t, second, "same", "same", "shadowed")
	writeSkill(t, second, "another", "same", "name shadowed")
	writeSkill(t, second, "different", "declared", "mismatch")

	registry, err := ScanRoots([]struct {
		Path   string
		Source string
	}{
		{Path: first, Source: "first"},
		{Path: second, Source: "second"},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := registry.Get("same")
	if !ok || entry.Location != firstLocation {
		t.Fatalf("same entry = %#v, ok=%v", entry, ok)
	}
	mismatch, ok := registry.Get("declared")
	if !ok || mismatch.AutoEnabled || mismatch.DirectoryName != "different" {
		t.Fatalf("mismatch entry = %#v, ok=%v", mismatch, ok)
	}
	diagnostics := registry.Diagnostics()
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want only mismatch warnings", diagnostics)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Code != "skill_name_mismatch" {
			t.Fatalf("diagnostics = %#v, found unexpected shadow warning", diagnostics)
		}
	}
}

func TestPromptSectionIncludesOnlyAutoEnabledEntries(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "usable", "usable", "Use < and & safely")
	writeSkill(t, root, "directory", "declared", "disabled")
	registry, err := ScanRoots([]struct {
		Path   string
		Source string
	}{
		{Path: root, Source: "test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt := registry.PromptSection()
	if !containsAll(prompt, "<name>usable</name>", "&lt;", "&amp;") || contains(prompt, "<name>declared</name>") {
		t.Fatalf("prompt = %s", prompt)
	}
}

func TestParseReferencesSkipsFencedCodeAndDeduplicates(t *testing.T) {
	text := "use [$one](/tmp/one/SKILL.md) and [$one](/tmp/one/SKILL.md) and `[$inline](/tmp/inline/SKILL.md)`\n```md\n[$two](/tmp/two/SKILL.md)\n```"
	references := ParseReferences(text)
	if len(references) != 1 || references[0].Name != "one" || references[0].Path != "/tmp/one/SKILL.md" {
		t.Fatalf("references = %#v", references)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !contains(value, part) {
			return false
		}
	}
	return true
}

func contains(value, part string) bool {
	return strings.Contains(value, part)
}
