package builtin

import (
	"path/filepath"
	"testing"
)

func TestResolvePathExpandsEnvironmentVariablesBeforeCheckingAbsolutePaths(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "cwd")
	absoluteRoot := filepath.Join(t.TempDir(), "absolute")
	t.Setenv("AGENTY_TEST_ABSOLUTE_ROOT", absoluteRoot)
	t.Setenv("AGENTY_TEST_RELATIVE_ROOT", "nested")
	t.Setenv("AGENTY_TEST_WINDOWS_ROOT", `C:\workspace`)

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "drive letter with backslashes",
			path: `C:\workspace\file.txt`,
			want: filepath.Clean(`C:\workspace\file.txt`),
		},
		{
			name: "drive letter with slashes",
			path: `C:/workspace/file.txt`,
			want: filepath.Clean(`C:/workspace/file.txt`),
		},
		{
			name: "UNC path",
			path: `\\server\share\file.txt`,
			want: filepath.Clean(`\\server\share\file.txt`),
		},
		{
			name: "braced POSIX environment variable",
			path: `${AGENTY_TEST_ABSOLUTE_ROOT}/file.txt`,
			want: filepath.Join(absoluteRoot, "file.txt"),
		},
		{
			name: "POSIX environment variable",
			path: `$AGENTY_TEST_ABSOLUTE_ROOT/file.txt`,
			want: filepath.Join(absoluteRoot, "file.txt"),
		},
		{
			name: "Windows environment variable",
			path: `%AGENTY_TEST_ABSOLUTE_ROOT%/file.txt`,
			want: filepath.Join(absoluteRoot, "file.txt"),
		},
		{
			name: "PowerShell environment variable",
			path: `$env:AGENTY_TEST_ABSOLUTE_ROOT/file.txt`,
			want: filepath.Join(absoluteRoot, "file.txt"),
		},
		{
			name: "relative environment variable",
			path: `${AGENTY_TEST_RELATIVE_ROOT}/file.txt`,
			want: filepath.Join(cwd, "nested", "file.txt"),
		},
		{
			name: "Windows environment variable expands to a drive path",
			path: `%AGENTY_TEST_WINDOWS_ROOT%\file.txt`,
			want: filepath.Clean(`C:\workspace\file.txt`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolvePath(test.path, cwd, false)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Errorf("resolvePath(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}
