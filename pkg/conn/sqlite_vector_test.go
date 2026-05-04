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

package conn

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

type fakeSQLiteExtensionLoader struct {
	lib   string
	entry string
}

func (l *fakeSQLiteExtensionLoader) LoadExtension(lib string, entry string) error {
	l.lib = lib
	l.entry = entry
	return nil
}

func TestSelectSQLiteVectorAsset(t *testing.T) {
	assets := []sqliteVectorAsset{
		{Name: "vector-linux-x86_64-0.9.95.tar.gz", BrowserDownloadURL: "https://example.test/linux.tar.gz"},
		{Name: "vector-linux-musl-x86_64-0.9.95.tar.gz", BrowserDownloadURL: "https://example.test/linux-musl.tar.gz"},
		{Name: "vector-macos-arm64-0.9.95.tar.gz", BrowserDownloadURL: "https://example.test/macos.tar.gz"},
		{Name: "vector-windows-x86_64-0.9.95.tar.gz", BrowserDownloadURL: "https://example.test/windows.tar.gz"},
	}

	asset, err := selectReleaseAsset(assets, "0.9.95", "linux", "amd64")
	if err != nil {
		t.Fatalf("select linux asset: %v", err)
	}
	if asset.Name != "vector-linux-x86_64-0.9.95.tar.gz" {
		t.Fatalf("unexpected linux asset: %s", asset.Name)
	}

	asset, err = selectReleaseAsset(assets, "v0.9.95", "darwin", "arm64")
	if err != nil {
		t.Fatalf("select macos asset: %v", err)
	}
	if asset.Name != "vector-macos-arm64-0.9.95.tar.gz" {
		t.Fatalf("unexpected macos asset: %s", asset.Name)
	}
}

func TestSelectSQLiteVectorAssetUnsupported(t *testing.T) {
	_, err := selectReleaseAsset(nil, "0.9.95", "plan9", "amd64")
	if err == nil {
		t.Fatal("expected unsupported OS error")
	}
}

func TestSQLiteVectorAssetNameFor(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		goarch  string
		version string
		libc    string
		want    string
	}{
		{name: "macos arm64", goos: "darwin", goarch: "arm64", version: "0.9.95", want: "vector-macos-arm64-0.9.95.tar.gz"},
		{name: "linux glibc amd64", goos: "linux", goarch: "amd64", version: "0.9.95", libc: "glibc", want: "vector-linux-x86_64-0.9.95.tar.gz"},
		{name: "linux musl arm64", goos: "linux", goarch: "arm64", version: "0.9.95", libc: "musl", want: "vector-linux-musl-arm64-0.9.95.tar.gz"},
		{name: "windows amd64", goos: "windows", goarch: "amd64", version: "0.9.95", want: "vector-windows-x86_64-0.9.95.tar.gz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sqliteVectorAssetNameFor(tt.goos, tt.goarch, tt.version, tt.libc)
			if err != nil {
				t.Fatalf("asset name: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestSQLiteVectorAssetNameForUnsupported(t *testing.T) {
	if _, err := sqliteVectorAssetNameFor("linux", "386", "0.9.95", "glibc"); err == nil {
		t.Fatal("expected 386 unsupported error")
	}
	if _, err := sqliteVectorAssetNameFor("windows", "arm64", "0.9.95", ""); err == nil {
		t.Fatal("expected windows arm64 unsupported error")
	}
}

func TestSQLiteExtensionLoadPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		goos string
		want string
	}{
		{name: "darwin suffix", path: "/tmp/vector.dylib", goos: "darwin", want: "/tmp/vector"},
		{name: "linux suffix", path: "/tmp/vector.so", goos: "linux", want: "/tmp/vector"},
		{name: "windows suffix", path: `C:\tmp\vector.dll`, goos: "windows", want: `C:\tmp\vector`},
		{name: "custom no suffix", path: "/tmp/sqlite_vector", goos: "darwin", want: "/tmp/sqlite_vector"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sqliteExtensionLoadPath(tt.path, tt.goos)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestLoadSQLiteVectorExtensionUsesExplicitEntryPoint(t *testing.T) {
	loader := &fakeSQLiteExtensionLoader{}
	err := loadExtension(loader, "/tmp/vector.dylib", "darwin")
	if err != nil {
		t.Fatalf("load extension: %v", err)
	}
	if loader.lib != "/tmp/vector" {
		t.Fatalf("expected suffix-stripped load path, got %q", loader.lib)
	}
	if loader.entry != sqliteVectorEntryPoint {
		t.Fatalf("expected entry point %q, got %q", sqliteVectorEntryPoint, loader.entry)
	}
}

func TestExtractSQLiteVectorTarGZ(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	content := []byte("extension")
	if err := tw.WriteHeader(&tar.Header{Name: "lib/vector.so", Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}

	var dst bytes.Buffer
	if err := extract(&archive, "vector-linux-x86_64-0.9.95.tar.gz", &dst, "vector.so"); err != nil {
		t.Fatalf("extract tar.gz: %v", err)
	}
	if !bytes.Equal(dst.Bytes(), content) {
		t.Fatalf("unexpected extracted content: %q", dst.String())
	}
}

func TestInstallVectorPluginFromReaderUsesAssetNameForExtraction(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	content := []byte("extension")
	if err := tw.WriteHeader(&tar.Header{Name: "lib/vector.dylib", Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}

	target := filepath.Join(t.TempDir(), "vector.dylib")
	err := installVectorPluginFromReader(&archive, "vector-macos-arm64-0.9.95.tar.gz", target)
	if err != nil {
		t.Fatalf("install plugin: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read installed plugin: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("expected extracted library content, got %x", got[:min(len(got), 8)])
	}
}

func TestExtractSQLiteVectorZip(t *testing.T) {
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	w, err := zw.Create("bin/vector.dylib")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	content := []byte("extension")
	if _, err = w.Write(content); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err = zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	var dst bytes.Buffer
	if err = extract(&archive, "vector-macos-arm64-0.9.95.zip", &dst, "vector.dylib"); err != nil {
		t.Fatalf("extract zip: %v", err)
	}
	if !bytes.Equal(dst.Bytes(), content) {
		t.Fatalf("unexpected extracted content: %q", dst.String())
	}
}
