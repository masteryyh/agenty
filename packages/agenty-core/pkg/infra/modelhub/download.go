package modelhub

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	domain "github.com/masteryyh/agenty-core/pkg/domain/localinference"
)

type PlanInput struct {
	Source          Source `json:"source"`
	Repository      string `json:"repository"`
	Variant         string `json:"variant"`
	CodecRepository string `json:"codecRepository,omitempty"`
}
type Plan struct {
	ID              string        `json:"id"`
	Source          Source        `json:"source"`
	Repository      string        `json:"repository"`
	Variant         string        `json:"variant"`
	CodecRepository string        `json:"codecRepository,omitempty"`
	Directory       string        `json:"directory"`
	Files           []File        `json:"files"`
	TotalBytes      int64         `json:"totalBytes"`
	RequiredBytes   int64         `json:"requiredBytes"`
	Storage         Storage       `json:"storage"`
	Supported       bool          `json:"supported"`
	Reason          string        `json:"reason,omitempty"`
	EnoughSpace     bool          `json:"enoughSpace"`
	Local           *domain.Model `json:"-"`
	ContextWindow   int           `json:"-"`
	created         time.Time
}
type FileProgress struct {
	Path            string `json:"path"`
	DownloadedBytes int64  `json:"downloadedBytes"`
	TotalBytes      int64  `json:"totalBytes"`
	Status          string `json:"status"`
}
type Download struct {
	ID              string         `json:"id"`
	Source          Source         `json:"source"`
	Repository      string         `json:"repository"`
	Variant         string         `json:"variant"`
	CodecRepository string         `json:"codecRepository,omitempty"`
	Directory       string         `json:"directory"`
	Status          string         `json:"status"`
	DownloadedBytes int64          `json:"downloadedBytes"`
	TotalBytes      int64          `json:"totalBytes"`
	Files           []FileProgress `json:"files"`
	Error           string         `json:"error,omitempty"`
	ModelCode       string         `json:"modelCode,omitempty"`
}
type downloadTask struct {
	state  Download
	cancel context.CancelFunc
}
type Options struct {
	Directory string
	Workers   func() []domain.Worker
	Publish   func(Download)
	Register  func(context.Context, string, string, int, *domain.Model) error
}
type Manager struct {
	Catalog *Catalog
	options Options
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	events  sync.Mutex
	wait    sync.WaitGroup
	plans   map[string]Plan
	tasks   map[string]*downloadTask
	space   func(string) (Storage, error)
}

func NewManager(ctx context.Context, options Options) *Manager {
	ctx, cancel := context.WithCancel(ctx)
	return &Manager{Catalog: NewCatalog(), options: options, ctx: ctx, cancel: cancel, plans: map[string]Plan{}, tasks: map[string]*downloadTask{}, space: diskSpace}
}
func (m *Manager) Storage() (Storage, error) { return m.space(m.options.Directory) }
func (m *Manager) Prepare(ctx context.Context, input PlanInput) (Plan, error) {
	d, err := m.Catalog.Inspect(ctx, input.Source, input.Repository)
	if err != nil {
		return Plan{}, err
	}
	index := slices.IndexFunc(d.Variants, func(v Variant) bool { return v.ID == input.Variant })
	if index < 0 {
		return Plan{}, fmt.Errorf("model variant not found; refresh the model details")
	}
	v := d.Variants[index]
	p := Plan{ID: uuid.NewString(), Source: input.Source, Repository: input.Repository, Variant: v.ID, Files: slices.Clone(v.Files), created: time.Now()}
	codec := d
	_, hasTokenizer := findFile(d.Files, "tokenizer.json")
	if !hasTokenizer || input.CodecRepository != "" {
		repo := input.CodecRepository
		if repo == "" {
			repo = d.BaseModel
		}
		if repo != "" && repo != d.ID {
			codec, err = m.Catalog.Inspect(ctx, input.Source, repo)
			if err != nil {
				if input.CodecRepository != "" {
					return Plan{}, fmt.Errorf("resolve tokenizer repository: %w", err)
				}
				// Keep a preview so the client can explain why this model is unsupported.
				codec = d
			}
		}
	}
	_, hasTokenizer = findFile(codec.Files, "tokenizer.json")
	codecPrefix := ""
	if codec.ID != d.ID {
		codecPrefix = "codec/"
		p.CodecRepository = codec.ID
	}
	ancillary := []string{"config.json", "tokenizer.json", "tokenizer_config.json", "chat_template.jinja", "chat_template.json", "special_tokens_map.json", "generation_config.json", "vocab.json", "merges.txt", "model.safetensors.index.json"}
	for _, file := range d.Files {
		if slices.Contains(ancillary, file.Path) {
			p.Files = append(p.Files, file)
		}
	}
	if codecPrefix != "" {
		for _, file := range codec.Files {
			if slices.Contains(ancillary, file.Path) && file.Path != "model.safetensors.index.json" {
				file.Path = codecPrefix + file.Path
				p.Files = append(p.Files, file)
			}
		}
	}
	slices.SortFunc(p.Files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	for _, f := range p.Files {
		if !safePath(f.Path) || f.Size <= 0 {
			return Plan{}, fmt.Errorf("invalid or unknown file size/path: %s", f.Path)
		}
		if f.Size > 1<<50 || p.TotalBytes > 1<<50-f.Size {
			return Plan{}, fmt.Errorf("model resource size exceeds supported range")
		}
		p.TotalBytes += f.Size
	}
	// URLs contain immutable revisions; their digest separates snapshots and quantizations.
	encoded, _ := json.Marshal(struct {
		Source     Source
		Repository string
		Variant    string
		Files      []string
	}{input.Source, input.Repository, v.ID, fileIdentities(p.Files)})
	digest := sha256.Sum256(encoded)
	p.Directory = filepath.Join(m.options.Directory, hex.EncodeToString(digest[:]))
	cfg := codec.Config.text()
	parser := ""
	switch codec.Config.family() {
	case "qwen3", "qwen3_moe":
		parser = "qwen3"
	case "qwen3_5", "qwen3_5_moe", "qwen3_5_text", "qwen3_5_moe_text":
		parser = "qwen3_5"
	}
	backend := domain.MLX
	if v.Format == "gguf" {
		backend = domain.LlamaCpp
	}
	p.Supported = true
	switch {
	case parser == "":
		p.Reason = "Agenty currently supports Qwen3 and Qwen3.5 output formats. This model's architecture is unsupported or unknown."
	case !hasTokenizer:
		p.Reason = "A matching tokenizer.json could not be found in this repository or its declared base model."
	case backend == domain.MLX && codecPrefix != "":
		p.Reason = "MLX requires tokenizer.json in the weights repository."
	case backend == domain.MLX && (runtime.GOOS != "darwin" || runtime.GOARCH != "arm64"):
		p.Reason = "Safetensors inference requires an MLX worker on Apple Silicon. Choose a GGUF model for llama.cpp."
	}
	if p.Reason == "" && m.options.Workers != nil && !slices.ContainsFunc(m.options.Workers(), func(w domain.Worker) bool { return w.Backend == backend && w.Enabled && w.Command != "" }) {
		p.Reason = fmt.Sprintf("Install and enable a %s worker before adding this model for inference.", backend)
	}
	p.Supported = p.Reason == ""
	weights := p.Directory
	if backend == domain.LlamaCpp {
		weights = filepath.Join(weights, filepath.FromSlash(v.Files[0].Path))
	}
	p.Local = &domain.Model{BackendPreference: "auto", OutputParser: parser, Resources: []domain.Resource{{ID: string(backend), Backend: backend, Format: v.Format, WeightsPath: weights, CodecDirectory: filepath.Join(p.Directory, filepath.FromSlash(codecPrefix))}}}
	p.ContextWindow = min(max(cfg.Context, 1024), 8192)
	if err := m.refreshSpace(ctx, &p); err != nil {
		return Plan{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ctx.Err(); err != nil {
		return Plan{}, err
	}
	for id, previous := range m.plans {
		if time.Since(previous.created) > 30*time.Minute {
			delete(m.plans, id)
		}
	}
	if len(m.plans) >= 32 {
		oldestID := ""
		var oldest time.Time
		for id, previous := range m.plans {
			if oldestID == "" || previous.created.Before(oldest) {
				oldestID, oldest = id, previous.created
			}
		}
		delete(m.plans, oldestID)
	}
	m.plans[p.ID] = p
	return p, nil
}
func fileIdentities(files []File) []string {
	result := make([]string, 0, len(files))
	for _, f := range files {
		result = append(result, fmt.Sprintf("%s:%d:%s:%s:%s", f.Path, f.Size, f.URL, f.SHA256, f.OID))
	}
	return result
}
func safePath(name string) bool {
	return !strings.ContainsAny(name, "\\:\x00") && filepath.IsLocal(filepath.FromSlash(name)) && filepath.ToSlash(filepath.Clean(name)) == name
}

// Every managed descendant must be a regular file/directory, never a symlink.
func safeDestination(root, name string) (string, error) {
	if !safePath(name) {
		return "", fmt.Errorf("unsafe model file path %q", name)
	}
	current := root
	for _, part := range strings.Split(name, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("model file path contains a symlink: %s", current)
		}
	}
	return current, nil
}
func fileValid(ctx context.Context, name string, file File) bool {
	info, err := os.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() != file.Size {
		return false
	}
	if file.SHA256 == "" && file.OID == "" {
		return true
	}
	f, err := os.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	var h hash.Hash = sha256.New()
	expected := file.SHA256
	if expected == "" {
		h = sha1.New()
		expected = file.OID
		_, _ = fmt.Fprintf(h, "blob %d\x00", file.Size)
	}
	buffer := make([]byte, 256<<10)
	for {
		if ctx.Err() != nil {
			return false
		}
		n, err := f.Read(buffer)
		_, _ = h.Write(buffer[:n])
		if err == io.EOF {
			return strings.EqualFold(hex.EncodeToString(h.Sum(nil)), expected)
		}
		if err != nil {
			return false
		}
	}
}
func (m *Manager) refreshSpace(ctx context.Context, p *Plan) error {
	storage, err := m.Storage()
	if err != nil {
		return fmt.Errorf("inspect model partition: %w", err)
	}
	p.Storage = storage
	p.RequiredBytes = 0
	for _, file := range p.Files {
		name, err := safeDestination(m.options.Directory, filepath.ToSlash(filepath.Join(filepath.Base(p.Directory), file.Path)))
		if err != nil {
			return err
		}
		if fileValid(ctx, name, file) {
			continue
		}
		remaining := file.Size
		part, err := os.Lstat(name + ".part")
		if err == nil && part.Mode().IsRegular() {
			remaining -= min(part.Size(), file.Size)
		}
		p.RequiredBytes += remaining
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	p.EnoughSpace = uint64(p.RequiredBytes) <= storage.AvailableBytes
	return nil
}
func active(status string) bool {
	return status == "downloading" || status == "checking" || status == "registering"
}
func cloneDownload(s Download) Download { s.Files = slices.Clone(s.Files); return s }
func (m *Manager) List() []Download {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Download, 0, len(m.tasks))
	for _, t := range m.tasks {
		result = append(result, cloneDownload(t.state))
	}
	slices.SortFunc(result, func(a, b Download) int { return strings.Compare(a.ID, b.ID) })
	return result
}
func (m *Manager) update(t *downloadTask, change func(*Download)) {
	m.events.Lock()
	defer m.events.Unlock()
	m.mu.Lock()
	change(&t.state)
	state := cloneDownload(t.state)
	m.mu.Unlock()
	if m.options.Publish != nil {
		m.options.Publish(state)
	}
}
func (m *Manager) Start(ctx context.Context, id string, downloadOnly bool) (Download, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.ctx.Err(); err != nil {
		return Download{}, err
	}
	p, ok := m.plans[id]
	if !ok || time.Since(p.created) > 30*time.Minute {
		return Download{}, fmt.Errorf("download preview expired; select the model again")
	}
	if !p.Supported {
		return Download{}, fmt.Errorf("unsupported model: %s", p.Reason)
	}
	if !downloadOnly && m.options.Register == nil {
		return Download{}, fmt.Errorf("model registration is unavailable")
	}
	for _, t := range m.tasks {
		if active(t.state.Status) {
			return Download{}, fmt.Errorf("another model download is in progress")
		}
	}
	// Recheck while admission is serialized, so another download cannot consume the same space.
	if err := m.refreshSpace(ctx, &p); err != nil {
		return Download{}, err
	}
	if !p.EnoughSpace {
		return Download{}, &SpaceError{Required: p.RequiredBytes, Available: p.Storage.AvailableBytes}
	}
	for id, t := range m.tasks {
		if len(m.tasks) >= 32 && !active(t.state.Status) {
			delete(m.tasks, id)
		}
	}
	workCtx, cancel := context.WithCancel(m.ctx)
	t := &downloadTask{state: Download{ID: uuid.NewString(), Source: p.Source, Repository: p.Repository, Variant: p.Variant, CodecRepository: p.CodecRepository, Directory: p.Directory, Status: "downloading", TotalBytes: p.TotalBytes, Files: []FileProgress{}}, cancel: cancel}
	for _, f := range p.Files {
		t.state.Files = append(t.state.Files, FileProgress{Path: f.Path, TotalBytes: f.Size, Status: "waiting"})
	}
	m.tasks[t.state.ID] = t
	state := cloneDownload(t.state)
	m.wait.Add(1)
	go func() {
		defer m.wait.Done()
		defer cancel()
		m.update(t, func(*Download) {})
		err := m.download(workCtx, t, p, downloadOnly)
		if err != nil {
			m.update(t, func(s *Download) {
				s.Status = "failed"
				if workCtx.Err() != nil {
					s.Status = "cancelled"
				}
				s.Error = err.Error()
			})
		}
	}()
	return state, nil
}

type SpaceError struct {
	Required  int64
	Available uint64
}

func (e *SpaceError) Error() string {
	return fmt.Sprintf("Insufficient disk space: need %d bytes, partition has %d bytes available. Free space and try again.", e.Required, e.Available)
}
func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.tasks[id]
	if t == nil {
		return fmt.Errorf("download not found")
	}
	if active(t.state.Status) {
		t.cancel()
	}
	return nil
}
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	m.cancel()
	m.mu.Unlock()
	done := make(chan struct{})
	go func() { m.wait.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (m *Manager) download(ctx context.Context, t *downloadTask, p Plan, downloadOnly bool) error {
	if err := os.MkdirAll(m.options.Directory, 0700); err != nil {
		return err
	}
	root, err := filepath.EvalSymlinks(m.options.Directory)
	if err != nil {
		return err
	}
	directory, err := safeDestination(root, filepath.Base(p.Directory))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	for i, file := range p.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		name, err := safeDestination(directory, file.Path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			return err
		}
		if err := m.downloadFile(ctx, file, name, func(count int64, status string) {
			m.update(t, func(s *Download) {
				s.Files[i].DownloadedBytes = count
				s.Files[i].Status = status
				s.DownloadedBytes = 0
				for _, f := range s.Files {
					s.DownloadedBytes += f.DownloadedBytes
				}
			})
		}); err != nil {
			return fmt.Errorf("download %s: %w", file.Path, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if downloadOnly {
		m.update(t, func(s *Download) { s.Status = "downloaded" })
		return nil
	}
	m.update(t, func(s *Download) { s.Status = "registering" })
	code := string(p.Source) + "/" + p.Repository + ":" + filepath.Base(p.Directory)[:12]
	if err := m.options.Register(ctx, code, p.Repository, p.ContextWindow, p.Local); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		m.update(t, func(s *Download) {
			s.Status = "downloaded"
			s.Error = "Files downloaded, but model preflight/registration failed: " + err.Error()
		})
		return nil
	}
	m.update(t, func(s *Download) {
		s.Status = "ready"
		s.ModelCode = code
	})
	return nil
}
func (m *Manager) downloadFile(ctx context.Context, file File, name string, progress func(int64, string)) error {
	if fileValid(ctx, name, file) {
		progress(file.Size, "complete")
		return nil
	}
	part := name + ".part"
	if info, err := os.Lstat(part); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("partial model file is not regular")
	}
	if fileValid(ctx, part, file) {
		if err := os.Rename(part, name); err != nil {
			return err
		}
		progress(file.Size, "complete")
		return nil
	}
	f, err := os.OpenFile(part, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	offset := info.Size()
	if offset >= file.Size {
		if err := f.Truncate(0); err != nil {
			return err
		}
		offset = 0
	}
	transferCtx, stopTransfer := context.WithCancelCause(ctx)
	defer stopTransfer(nil)
	idle := time.AfterFunc(time.Minute, func() {
		stopTransfer(fmt.Errorf("model download stalled for one minute; retry to resume"))
	})
	defer idle.Stop()
	req, err := http.NewRequestWithContext(transferCtx, http.MethodGet, file.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept-Encoding", "identity")
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 90 * time.Second}}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		if transferCtx.Err() != nil {
			return context.Cause(transferCtx)
		}
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		if err := f.Truncate(0); err != nil {
			return err
		}
		offset = 0
	case http.StatusPartialContent:
		var start, end, total int64
		if _, err := fmt.Sscanf(resp.Header.Get("Content-Range"), "bytes %d-%d/%d", &start, &end, &total); err != nil || start != offset || total != file.Size || end != total-1 {
			return fmt.Errorf("invalid download Content-Range")
		}
	default:
		return fmt.Errorf("model hub returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength >= 0 && resp.ContentLength != file.Size-offset {
		return fmt.Errorf("download size changed; refresh the preview")
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	progress(offset, "downloading")
	last := time.Now()
	buffer := make([]byte, 256<<10)
	for {
		if transferCtx.Err() != nil {
			return context.Cause(transferCtx)
		}
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			idle.Reset(time.Minute)
			if int64(n) > file.Size-offset {
				return fmt.Errorf("download exceeds expected size")
			}
			if _, err := f.Write(buffer[:n]); err != nil {
				return err
			}
			offset += int64(n)
			if time.Since(last) >= 150*time.Millisecond {
				progress(offset, "downloading")
				last = time.Now()
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			if transferCtx.Err() != nil {
				return context.Cause(transferCtx)
			}
			return readErr
		}
	}
	if offset != file.Size {
		return fmt.Errorf("incomplete download: %d of %d bytes", offset, file.Size)
	}
	idle.Stop()
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	progress(offset, "checking")
	if !fileValid(ctx, part, file) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_ = os.Remove(part)
		return fmt.Errorf("model file checksum mismatch; retry to download a fresh copy")
	}
	if err := os.Rename(part, name); err != nil {
		return err
	}
	progress(offset, "complete")
	return nil
}
