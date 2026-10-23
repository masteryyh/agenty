package localinference

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	domain "github.com/masteryyh/agenty-core/pkg/domain/localinference"
)

type BundleManifest struct {
	SchemaVersion int            `json:"schemaVersion"`
	ID            string         `json:"id"`
	Backend       domain.Backend `json:"backend"`
	Version       string         `json:"version"`
	OS            string         `json:"os"`
	Arch          string         `json:"arch"`
	Command       string         `json:"command"`
	Arguments     []string       `json:"arguments"`
}
type InstallInput struct {
	URL      string `json:"url,omitempty"`
	FilePath string `json:"filePath,omitempty"`
	SHA256   string `json:"sha256"`
}
type Installation struct {
	ID              string         `json:"id"`
	Status          string         `json:"status"`
	DownloadedBytes int64          `json:"downloadedBytes"`
	TotalBytes      int64          `json:"totalBytes"`
	Error           string         `json:"error,omitempty"`
	Worker          *domain.Worker `json:"worker,omitempty"`
}
type installTask struct {
	state  Installation
	cancel context.CancelFunc
}
type Installer struct {
	ctx       context.Context
	cancel    context.CancelFunc
	wait      sync.WaitGroup
	directory string
	mu        sync.Mutex
	tasks     map[string]*installTask
	publish   func(Installation)
	activate  func(domain.Worker) error
}

func NewInstaller(ctx context.Context, directory string, publish func(Installation), activate func(domain.Worker) error) *Installer {
	ctx, cancel := context.WithCancel(ctx)
	return &Installer{ctx: ctx, cancel: cancel, directory: directory, tasks: make(map[string]*installTask), publish: publish, activate: activate}
}

func (i *Installer) Shutdown(ctx context.Context) error {
	i.mu.Lock()
	i.cancel()
	i.mu.Unlock()
	done := make(chan struct{})
	go func() { i.wait.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (i *Installer) List() []Installation {
	i.mu.Lock()
	defer i.mu.Unlock()
	result := make([]Installation, 0, len(i.tasks))
	for _, task := range i.tasks {
		result = append(result, task.state)
	}
	return result
}
func (i *Installer) update(task *installTask, change func(*Installation)) {
	i.mu.Lock()
	change(&task.state)
	state := task.state
	i.mu.Unlock()
	if i.publish != nil {
		i.publish(state)
	}
}
func (i *Installer) Start(input InstallInput) (Installation, error) {
	if (input.URL == "") == (input.FilePath == "") {
		return Installation{}, fmt.Errorf("provide exactly one worker bundle URL or filePath")
	}
	if input.FilePath != "" {
		if !filepath.IsAbs(input.FilePath) {
			return Installation{}, fmt.Errorf("worker bundle filePath must be absolute")
		}
	} else {
		parsed, err := url.Parse(input.URL)
		if err != nil || parsed.Host == "" || parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost")) {
			return Installation{}, fmt.Errorf("worker bundle URL must use HTTPS")
		}
	}
	digest, err := hex.DecodeString(input.SHA256)
	if err != nil || len(digest) != 32 {
		return Installation{}, fmt.Errorf("bundle SHA-256 must be 64 hexadecimal characters")
	}
	i.mu.Lock()
	if err := i.ctx.Err(); err != nil {
		i.mu.Unlock()
		return Installation{}, err
	}
	active := 0
	for _, task := range i.tasks {
		if task.state.Status == "downloading" || task.state.Status == "installing" {
			active++
		}
	}
	if active > 0 {
		i.mu.Unlock()
		return Installation{}, fmt.Errorf("another worker installation is in progress")
	}
	if len(i.tasks) >= 32 {
		for id, task := range i.tasks {
			if task.state.Status != "downloading" && task.state.Status != "installing" {
				delete(i.tasks, id)
			}
		}
	}
	ctx, cancel := context.WithTimeout(i.ctx, 30*time.Minute)
	task := &installTask{state: Installation{ID: uuid.NewString(), Status: "downloading"}, cancel: cancel}
	if input.FilePath != "" {
		task.state.Status = "installing"
	}
	i.tasks[task.state.ID] = task
	state := task.state
	i.wait.Add(1)
	i.mu.Unlock()
	if i.publish != nil {
		i.publish(state)
	}
	go func() {
		defer i.wait.Done()
		defer cancel()
		err := i.install(ctx, task, input)
		if err != nil {
			i.update(task, func(s *Installation) {
				s.Status = "failed"
				if ctx.Err() != nil {
					s.Status = "cancelled"
				}
				s.Error = err.Error()
			})
		}
	}()
	return state, nil
}
func (i *Installer) Cancel(id string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	task := i.tasks[id]
	if task == nil {
		return fmt.Errorf("installation not found")
	}
	task.cancel()
	return nil
}
func (i *Installer) install(ctx context.Context, task *installTask, input InstallInput) error {
	if err := os.MkdirAll(i.directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(i.directory, ".bundle-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	source, length, err := openBundleSource(ctx, input)
	if err != nil {
		_ = file.Close()
		return err
	}
	defer source.Close()
	i.update(task, func(s *Installation) { s.TotalBytes = length })
	hash := sha256.New()
	buffer := make([]byte, 256<<10)
	var total, lastProgress int64
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return err
		}
		n, readErr := source.Read(buffer)
		if n > 0 {
			total += int64(n)
			if total > 2<<30 {
				_ = file.Close()
				return fmt.Errorf("worker bundle exceeds 2 GiB")
			}
			if _, err := file.Write(buffer[:n]); err != nil {
				_ = file.Close()
				return err
			}
			_, _ = hash.Write(buffer[:n])
			if total-lastProgress >= 1<<20 {
				lastProgress = total
				i.update(task, func(s *Installation) { s.DownloadedBytes = total })
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = file.Close()
			return readErr
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), input.SHA256) {
		return fmt.Errorf("worker bundle SHA-256 mismatch")
	}
	i.update(task, func(s *Installation) { s.Status = "installing"; s.DownloadedBytes = total })
	staging, err := os.MkdirTemp(i.directory, ".install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	archive, err := zip.OpenReader(file.Name())
	if err != nil {
		return err
	}
	defer archive.Close()
	var expanded uint64
	for _, entry := range archive.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.FromSlash(entry.Name)
		if !filepath.IsLocal(path) || entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("invalid worker bundle path %q", entry.Name)
		}
		expanded += entry.UncompressedSize64
		if expanded > 4<<30 {
			return fmt.Errorf("expanded bundle exceeds 4 GiB")
		}
		destination := filepath.Join(staging, path)
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(destination, 0700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return err
		}
		reader, err := entry.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, entry.Mode().Perm()|0600)
		if err != nil {
			_ = reader.Close()
			return err
		}
		_, copyErr := io.Copy(output, io.LimitReader(reader, int64(entry.UncompressedSize64)+1))
		closeErr := output.Close()
		_ = reader.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	metadataFile := filepath.Join(staging, "worker.json")
	metadataInfo, err := os.Stat(metadataFile)
	if err != nil {
		return err
	}
	if metadataInfo.Size() > 1<<20 {
		return fmt.Errorf("worker manifest exceeds 1 MiB")
	}
	data, err := os.ReadFile(metadataFile)
	if err != nil {
		return err
	}
	var manifest BundleManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	if manifest.SchemaVersion != 1 || manifest.OS != runtime.GOOS || manifest.Arch != runtime.GOARCH || manifest.Version == "" || !filepath.IsLocal(manifest.Command) {
		return fmt.Errorf("worker bundle platform, version or executable is invalid")
	}
	if _, err := os.Stat(filepath.Join(staging, manifest.Command)); err != nil {
		return err
	}
	destination := filepath.Join(i.directory, input.SHA256)
	if err := os.Rename(staging, destination); err != nil {
		if _, statErr := os.Stat(filepath.Join(destination, "worker.json")); statErr != nil {
			return err
		}
	}
	arguments := make([]string, len(manifest.Arguments))
	for n, arg := range manifest.Arguments {
		arguments[n] = strings.ReplaceAll(arg, "@ROOT@", destination)
	}
	worker := domain.Worker{ID: manifest.ID, Backend: manifest.Backend, Version: manifest.Version, Enabled: true,
		Command: filepath.Join(destination, manifest.Command), Arguments: arguments}
	if err := (domain.Config{Workers: []domain.Worker{worker}}).Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := i.activate(worker); err != nil {
		return err
	}
	i.update(task, func(s *Installation) { s.Status = "installed"; s.Worker = &worker })
	return nil
}

func openBundleSource(ctx context.Context, input InstallInput) (io.ReadCloser, int64, error) {
	if input.FilePath != "" {
		info, err := os.Stat(input.FilePath)
		if err != nil {
			return nil, 0, err
		}
		if !info.Mode().IsRegular() || info.Size() > 2<<30 {
			return nil, 0, fmt.Errorf("local worker bundle must be a regular file no larger than 2 GiB")
		}
		file, err := os.Open(input.FilePath)
		if err != nil {
			return nil, 0, fmt.Errorf("open local worker bundle: %w", err)
		}
		return file, info.Size(), nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
	if err != nil {
		return nil, 0, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, 0, err
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return nil, 0, fmt.Errorf("download worker: HTTP %d", response.StatusCode)
	}
	return response.Body, response.ContentLength, nil
}
