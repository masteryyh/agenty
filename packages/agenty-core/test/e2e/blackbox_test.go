//go:build e2e

package e2e_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestE2ESystemDoesNotImportCoreImplementationPackages(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(filepath.Join(moduleRoot, "test", "e2e"))
	requireNoError(t, err)
	files := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		path := filepath.Join(moduleRoot, "test", "e2e", entry.Name())
		file, err := parser.ParseFile(
			files,
			path,
			nil,
			parser.ImportsOnly,
		)
		requireNoError(t, err)
		for _, importSpec := range file.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			requireNoError(t, err)
			if strings.HasPrefix(importPath, "github.com/masteryyh/agenty-core/") {
				t.Errorf("%s imports core implementation package %q", entry.Name(), importPath)
			}
		}
	}
}
