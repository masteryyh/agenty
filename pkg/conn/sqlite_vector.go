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
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/masteryyh/agenty/pkg/consts"
)

const sqliteVectorEntryPoint = "sqlite3_vector_init"

type sqliteVectorRelease struct {
	TagName string              `json:"tag_name"`
	Assets  []sqliteVectorAsset `json:"assets"`
}

type sqliteVectorAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func loadSQLiteVector(ctx context.Context, sqlDB *sql.DB, configuredPath, configDir string) error {
	extensionPath := sqliteVectorExtensionPath(configuredPath, configDir)
	if err := ensureExtension(ctx, extensionPath); err != nil {
		return err
	}

	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	return conn.Raw(func(driverConn any) error {
		if err := loadExtension(driverConn, extensionPath, runtime.GOOS); err != nil {
			return fmt.Errorf("failed to load sqlite-vector extension from %s: %w", extensionPath, err)
		}
		return nil
	})
}

func sqliteVectorExtensionPath(configuredPath, configDir string) string {
	if configuredPath != "" {
		return configuredPath
	}
	return filepath.Join(configDir, "vector"+extensionSuffix(runtime.GOOS))
}

func ensureExtension(ctx context.Context, extensionPath string) error {
	if _, err := os.Stat(extensionPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to inspect sqlite-vector extension at %s: %w", extensionPath, err)
	}

	release, err := fetchLatestRelease(ctx)
	if err != nil {
		return fmt.Errorf("sqlite-vector extension is missing at %s and latest release lookup failed: %w", extensionPath, err)
	}
	asset, err := selectReleaseAsset(release.Assets, release.TagName, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return fmt.Errorf("sqlite-vector extension is missing at %s and no release asset matches %s/%s in %s: %w", extensionPath, runtime.GOOS, runtime.GOARCH, release.TagName, err)
	}

	slog.InfoContext(ctx, "downloading sqlite-vector extension", "version", release.TagName, "asset", asset.Name, "target", extensionPath)
	if err = installVectorPlugin(ctx, asset.BrowserDownloadURL, asset.Name, extensionPath); err != nil {
		return fmt.Errorf("failed to download sqlite-vector %s for %s/%s: %w", release.TagName, runtime.GOOS, runtime.GOARCH, err)
	}
	return nil
}

func fetchLatestRelease(ctx context.Context) (*sqliteVectorRelease, error) {
	release, err := Get[sqliteVectorRelease](ctx, HTTPRequest{
		URL:     consts.SQLiteVectorLatestReleaseURL,
		Headers: map[string]string{"Accept": "application/vnd.github+json", "User-Agent": "agenty"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest sqlite-vector release: %w", err)
	}

	if release.TagName == "" || len(release.Assets) == 0 {
		return nil, fmt.Errorf("latest release response has no downloadable assets")
	}
	return &release, nil
}

func selectReleaseAsset(assets []sqliteVectorAsset, tagName, goos, goarch string) (*sqliteVectorAsset, error) {
	version := sqliteVectorVersion(tagName)
	assetName, err := sqliteVectorAssetName(goos, goarch, version)
	if err != nil {
		return nil, err
	}

	for i := range assets {
		if strings.EqualFold(assets[i].Name, assetName) && assets[i].BrowserDownloadURL != "" {
			return &assets[i], nil
		}
	}
	return nil, fmt.Errorf("expected asset %s", assetName)
}

func sqliteVectorVersion(tagName string) string {
	return strings.TrimPrefix(tagName, "v")
}

func sqliteVectorAssetName(goos, goarch, version string) (string, error) {
	libc := ""
	if goos == "linux" {
		libc = detectCLib()
	}
	return sqliteVectorAssetNameFor(goos, goarch, version, libc)
}

func sqliteVectorAssetNameFor(goos, goarch, version, libc string) (string, error) {
	osTag, err := sqliteVectorOSTag(goos, libc)
	if err != nil {
		return "", err
	}
	archTag, err := sqliteVectorArchTag(goos, goarch)
	if err != nil {
		return "", err
	}
	if version == "" {
		return "", fmt.Errorf("empty sqlite-vector version")
	}
	return fmt.Sprintf("vector-%s-%s-%s.tar.gz", osTag, archTag, version), nil
}

func sqliteVectorOSTag(goos, libc string) (string, error) {
	switch goos {
	case "darwin":
		return "macos", nil
	case "linux":
		if libc == "musl" {
			return "linux-musl", nil
		}
		return "linux", nil
	case "windows":
		return "windows", nil
	default:
		return "", fmt.Errorf("unsupported OS: %s", goos)
	}
}

func sqliteVectorArchTag(goos, goarch string) (string, error) {
	if goos == "windows" && goarch == "arm64" {
		return "", fmt.Errorf("sqlite-vector does not provide windows arm64 assets")
	}
	switch goarch {
	case "amd64":
		return "x86_64", nil
	case "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s", goarch)
	}
}

func detectCLib() string {
	for _, pattern := range []string{
		"/lib/ld-musl-*.so.1",
		"/usr/lib/ld-musl-*.so.1",
		"/lib/*/ld-musl-*.so.1",
		"/usr/lib/*/ld-musl-*.so.1",
	} {
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) > 0 {
			return "musl"
		}
	}

	for _, path := range []string{"/proc/self/maps", "/usr/bin/ldd", "/bin/ldd"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := strings.ToLower(string(data))
		if strings.Contains(content, "musl") {
			return "musl"
		}
		if strings.Contains(content, "glibc") || strings.Contains(content, "gnu libc") {
			return "glibc"
		}
	}
	return "glibc"
}

func extensionSuffix(goos string) string {
	switch goos {
	case "darwin":
		return ".dylib"
	case "windows":
		return ".dll"
	default:
		return ".so"
	}
}

func sqliteExtensionLoadPath(extensionPath, goos string) string {
	suffix := extensionSuffix(goos)
	if strings.HasSuffix(strings.ToLower(extensionPath), suffix) {
		return extensionPath[:len(extensionPath)-len(suffix)]
	}
	return extensionPath
}

func loadExtension(driverConn any, extensionPath, goos string) error {
	loader, ok := driverConn.(interface {
		LoadExtension(string, string) error
	})
	if !ok {
		return fmt.Errorf("sqlite driver does not support loading extensions")
	}
	return loader.LoadExtension(sqliteExtensionLoadPath(extensionPath, goos), sqliteVectorEntryPoint)
}

func installVectorPlugin(ctx context.Context, url, assetName, extensionPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "agenty")

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("download returned %s", resp.Status)
	}

	if err = os.MkdirAll(filepath.Dir(extensionPath), 0o700); err != nil {
		return err
	}

	return installVectorPluginFromReader(resp.Body, assetName, extensionPath)
}

func installVectorPluginFromReader(r io.Reader, assetName, extensionPath string) error {
	tmpFile, err := os.CreateTemp(filepath.Dir(extensionPath), ".sqlite-vector-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if err = extract(r, assetName, tmpFile, filepath.Base(extensionPath)); err != nil {
		tmpFile.Close()
		return err
	}
	if err = tmpFile.Chmod(0o755); err != nil {
		tmpFile.Close()
		return err
	}
	if err = tmpFile.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, extensionPath)
}

func extract(r io.Reader, sourcePath string, dst io.Writer, targetBase string) error {
	sourcePath = strings.ToLower(sourcePath)
	switch {
	case strings.HasSuffix(sourcePath, ".tar.gz"):
		return extractTarGz(r, dst, targetBase)
	case strings.HasSuffix(sourcePath, ".zip"):
		return extractZip(r, dst, targetBase)
	default:
		_, err := io.Copy(dst, io.LimitReader(r, 128<<20))
		return err
	}
}

func extractTarGz(r io.Reader, dst io.Writer, targetBase string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if !isSQLiteVectorLibrary(header.Name, targetBase) {
			continue
		}
		_, err = io.Copy(dst, io.LimitReader(tr, 128<<20))
		return err
	}
	return fmt.Errorf("archive does not contain sqlite-vector library")
}

func extractZip(r io.Reader, dst io.Writer, targetBase string) error {
	data, err := io.ReadAll(io.LimitReader(r, 128<<20))
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, file := range zr.File {
		if file.FileInfo().IsDir() || !isSQLiteVectorLibrary(file.Name, targetBase) {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(dst, io.LimitReader(rc, 128<<20))
		closeErr := rc.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
	return fmt.Errorf("archive does not contain sqlite-vector library")
}

func isSQLiteVectorLibrary(name, targetBase string) bool {
	base := strings.ToLower(filepath.Base(name))
	targetBase = strings.ToLower(targetBase)
	if base == targetBase {
		return true
	}
	if !strings.Contains(base, "vector") {
		return false
	}
	return strings.HasSuffix(base, ".so") || strings.HasSuffix(base, ".dylib") || strings.HasSuffix(base, ".dll")
}
