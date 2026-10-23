package localinference

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	json "github.com/bytedance/sonic"

	domain "github.com/masteryyh/agenty-core/pkg/domain/localinference"
)

func TestInstallerRejectsChecksumAndArchiveTraversal(t *testing.T) {
	for _, test := range []struct {
		name, path string
		badHash    bool
	}{
		{"checksum", "worker.json", true}, {"traversal", "../escaped", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var buffer bytes.Buffer
			archive := zip.NewWriter(&buffer)
			file, err := archive.Create(test.path)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = file.Write([]byte("{}"))
			_ = archive.Close()
			digest := sha256.Sum256(buffer.Bytes())
			hash := hex.EncodeToString(digest[:])
			if test.badHash {
				hash = strings.Repeat("0", 64)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(buffer.Bytes()) }))
			defer server.Close()
			updates := make(chan Installation, 16)
			installer := NewInstaller(t.Context(), t.TempDir(), func(state Installation) { updates <- state }, func(domain.Worker) error { t.Error("invalid bundle activated"); return nil })
			if _, err := installer.Start(InstallInput{URL: server.URL, SHA256: hash}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			for {
				select {
				case state := <-updates:
					if state.Status == "failed" {
						if state.Error == "" {
							t.Fatal("missing failure reason")
						}
						return
					}
				case <-ctx.Done():
					t.Fatal("installation did not finish")
				}
			}
		})
	}
}

func TestLocalBundleInstallsWithoutDownloadAndValidatesHash(t *testing.T) {
	var data bytes.Buffer
	archive := zip.NewWriter(&data)
	manifest, _ := json.Marshal(BundleManifest{SchemaVersion: 1, ID: "local-native", Backend: domain.LlamaCpp, Version: "dev", OS: runtime.GOOS, Arch: runtime.GOARCH, Command: "bin/worker"})
	for name, content := range map[string][]byte{"worker.json": manifest, "bin/worker": []byte("local worker bytes")} {
		file, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "worker.zip")
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data.Bytes())
	hash := hex.EncodeToString(digest[:])
	for _, badHash := range []bool{false, true} {
		t.Run(map[bool]string{true: "checksum", false: "install"}[badHash], func(t *testing.T) {
			directory := t.TempDir()
			updates := make(chan Installation, 16)
			activated := make(chan domain.Worker, 1)
			installer := NewInstaller(t.Context(), directory, func(state Installation) { updates <- state }, func(worker domain.Worker) error { activated <- worker; return nil })
			input := InstallInput{FilePath: path, SHA256: hash}
			if badHash {
				input.SHA256 = strings.Repeat("0", 64)
			}
			state, err := installer.Start(input)
			if err != nil {
				t.Fatal(err)
			}
			if state.Status != "installing" {
				t.Fatalf("local archive was treated as a download: %+v", state)
			}
			for {
				select {
				case state := <-updates:
					if state.Status == "failed" {
						if !badHash || !strings.Contains(state.Error, "SHA-256") {
							t.Fatal(state.Error)
						}
						select {
						case <-activated:
							t.Fatal("bad hash activated")
						default:
						}
						return
					}
					if state.Status == "installed" {
						worker := <-activated
						if worker.Command != filepath.Join(directory, hash, "bin", "worker") || worker.Version != "dev" {
							t.Fatalf("wrong install destination: %+v", worker)
						}
						contents, err := os.ReadFile(worker.Command)
						if err != nil || string(contents) != "local worker bytes" {
							t.Fatal("local archive was not installed", err)
						}
						return
					}
				case <-time.After(3 * time.Second):
					t.Fatal("local installation did not finish")
				}
			}
		})
	}
	installer := NewInstaller(t.Context(), t.TempDir(), nil, nil)
	for _, input := range []InstallInput{{FilePath: "relative.zip", SHA256: hash}, {URL: "https://example.com/worker.zip", FilePath: path, SHA256: hash}} {
		if _, err := installer.Start(input); err == nil {
			t.Fatal("invalid local source accepted")
		}
	}
}

func TestInstallerShutdownCancelsDownloadAndRejectsNewTasks(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	installer := NewInstaller(t.Context(), t.TempDir(), nil, func(domain.Worker) error { t.Error("cancelled bundle was activated"); return nil })
	input := InstallInput{URL: server.URL, SHA256: strings.Repeat("0", 64)}
	if _, err := installer.Start(input); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("download did not start")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := installer.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if tasks := installer.List(); len(tasks) != 1 || tasks[0].Status != "cancelled" {
		t.Fatalf("download still active: %+v", tasks)
	}
	if _, err := installer.Start(input); err == nil {
		t.Fatal("installer accepted work after shutdown")
	}
}
