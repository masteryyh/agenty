package modelhub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	domain "github.com/masteryyh/agenty-core/pkg/domain/localinference"
)

func fixture(t *testing.T, source Source, register func(context.Context, string, string, int, *domain.Model) error) (*Manager, map[string]string) {
	t.Helper()
	files := map[string]string{"model-Q4.gguf": "model-weights", "config.json": `{"model_type":"qwen3","max_position_embeddings":32768}`, "tokenizer.json": `{"model":{"type":"BPE"}}`, "tokenizer_config.json": `{"chat_template":"template"}`}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/models/acme/model" || r.URL.Path == "/api/v1/models/acme/model":
			if source == ModelScope {
				_ = json.NewEncoder(w).Encode(map[string]any{"Code": 200, "Data": map[string]any{"BaseModel": []string{}}})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"sha": "revision", "safetensors": map[string]any{"total": 1230000000}})
			}
		case strings.Contains(r.URL.Path, "/tree/"):
			entries := []map[string]any{}
			for name, body := range files {
				sum := sha256.Sum256([]byte(body))
				entries = append(entries, map[string]any{"type": "file", "path": name, "size": len(body), "lfs": map[string]any{"oid": hex.EncodeToString(sum[:])}})
			}
			_ = json.NewEncoder(w).Encode(entries)
		case strings.HasSuffix(r.URL.Path, "/repo/files"):
			entries := []map[string]any{}
			for name, body := range files {
				sum := sha256.Sum256([]byte(body))
				entries = append(entries, map[string]any{"Type": "blob", "Path": name, "Size": len(body), "Sha256": hex.EncodeToString(sum[:]), "Revision": "revision"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"Code": 200, "Data": map[string]any{"Files": entries}})
		default:
			name := r.URL.Query().Get("FilePath")
			if name == "" {
				_, name, _ = strings.Cut(r.URL.Path, "/resolve/revision/")
			}
			body, ok := files[name]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			http.ServeContent(w, r, name, time.Time{}, strings.NewReader(body))
		}
	}))
	t.Cleanup(server.Close)
	m := NewManager(context.Background(), Options{Directory: filepath.Join(t.TempDir(), "models"), Workers: func() []domain.Worker {
		return []domain.Worker{{Backend: domain.LlamaCpp, Enabled: true, Command: "/worker"}}
	}, Register: register})
	m.Catalog.hf, m.Catalog.ms = server.URL, server.URL
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := m.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	return m, files
}
func prepare(t *testing.T, m *Manager, source Source) Plan {
	t.Helper()
	p, err := m.Prepare(context.Background(), PlanInput{Source: source, Repository: "acme/model", Variant: "model-Q4.gguf"})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func waitDownload(t *testing.T, m *Manager, id string) Download {
	t.Helper()
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, s := range m.List() {
			if s.ID == id && !active(s.Status) {
				return s
			}
		}
		select {
		case <-deadline:
			t.Fatal("download did not complete")
			return Download{}
		case <-ticker.C:
		}
	}
}
func TestDownloadBothSourcesAndRegister(t *testing.T) {
	for _, source := range []Source{HuggingFace, ModelScope} {
		t.Run(string(source), func(t *testing.T) {
			var registered atomic.Int32
			m, files := fixture(t, source, func(ctx context.Context, code, name string, window int, model *domain.Model) error {
				if model.OutputParser != "qwen3" || window != 8192 || !strings.HasPrefix(code, string(source)+"/") {
					return fmt.Errorf("invalid registration: %s %d %+v", code, window, model)
				}
				for _, resource := range model.Resources {
					if _, err := os.Stat(resource.WeightsPath); err != nil {
						return err
					}
					if _, err := os.Stat(filepath.Join(resource.CodecDirectory, "tokenizer.json")); err != nil {
						return err
					}
				}
				registered.Add(1)
				return nil
			})
			p := prepare(t, m, source)
			if !p.Supported || !p.EnoughSpace || len(p.Files) != len(files) {
				t.Fatalf("unexpected preview: %+v", p)
			}
			if _, err := os.Stat(m.options.Directory); !os.IsNotExist(err) {
				t.Fatalf("preview wrote to disk: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			s, err := m.Start(ctx, p.ID, false)
			cancel()
			if err != nil {
				t.Fatal(err)
			}
			s = waitDownload(t, m, s.ID)
			if s.Status != "ready" || registered.Load() != 1 || s.DownloadedBytes != s.TotalBytes {
				t.Fatalf("download result: %+v", s)
			}
			for _, f := range s.Files {
				if f.Status != "complete" {
					t.Fatalf("file not complete: %+v", f)
				}
			}
			p = prepare(t, m, source)
			if p.RequiredBytes != 0 {
				t.Fatalf("verified existing files counted again: %d", p.RequiredBytes)
			}
		})
	}
}
func TestSpaceRecheckedAndPartialsCounted(t *testing.T) {
	m, _ := fixture(t, HuggingFace, nil)
	var available atomic.Uint64
	available.Store(1 << 30)
	m.space = func(string) (Storage, error) { return Storage{AvailableBytes: available.Load()}, nil }
	p := prepare(t, m, HuggingFace)
	available.Store(0)
	_, err := m.Start(context.Background(), p.ID, true)
	var space *SpaceError
	if !errors.As(err, &space) || len(m.List()) != 0 {
		t.Fatalf("low space accepted: %v", err)
	}
	available.Store(1 << 30)
	if err := os.MkdirAll(p.Directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Directory, "model-Q4.gguf.part"), []byte("model-"), 0600); err != nil {
		t.Fatal(err)
	}
	partial := prepare(t, m, HuggingFace)
	if partial.RequiredBytes != p.TotalBytes-6 {
		t.Fatalf("partial counted incorrectly: %d of %d", partial.RequiredBytes, p.TotalBytes)
	}
	s, err := m.Start(context.Background(), partial.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if result := waitDownload(t, m, s.ID); result.Status != "downloaded" {
		t.Fatalf("resume failed: %+v", result)
	}
}
func TestUnsupportedAndPreflightFailure(t *testing.T) {
	m, files := fixture(t, HuggingFace, func(context.Context, string, string, int, *domain.Model) error {
		return errors.New("worker cannot load weights")
	})
	files["config.json"] = `{"model_type":"llama"}`
	p := prepare(t, m, HuggingFace)
	if p.Supported || p.Reason == "" {
		t.Fatal("unsupported model was accepted")
	}
	for _, downloadOnly := range []bool{false, true} {
		if _, err := m.Start(context.Background(), p.ID, downloadOnly); err == nil {
			t.Fatal("unsupported model download was accepted")
		}
	}
	if len(m.List()) != 0 {
		t.Fatal("unsupported model created a download task")
	}
	files["config.json"] = `{"model_type":"qwen3"}`
	p = prepare(t, m, HuggingFace)
	s, err := m.Start(context.Background(), p.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if result := waitDownload(t, m, s.ID); result.Status != "downloaded" || !strings.Contains(result.Error, "cannot load") {
		t.Fatalf("preflight failure lost files: %+v", result)
	}
}
func TestResumeProtocolAndChecksum(t *testing.T) {
	for _, mode := range []string{"range", "ignore-range", "bad-range", "corrupt"} {
		t.Run(mode, func(t *testing.T) {
			body := "model-content"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "range":
					http.ServeContent(w, r, "weights", time.Time{}, strings.NewReader(body))
				case "ignore-range":
					fmt.Fprint(w, body)
				case "bad-range":
					w.Header().Set("Content-Range", "bytes 0-12/13")
					w.WriteHeader(206)
					fmt.Fprint(w, body)
				case "corrupt":
					fmt.Fprint(w, strings.Repeat("x", len(body)))
				}
			}))
			defer server.Close()
			sum := sha256.Sum256([]byte(body))
			f := File{Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), URL: server.URL}
			name := filepath.Join(t.TempDir(), "weights")
			if err := os.WriteFile(name+".part", []byte(body[:4]), 0600); err != nil {
				t.Fatal(err)
			}
			m := NewManager(context.Background(), Options{})
			defer m.cancel()
			err := m.downloadFile(context.Background(), f, name, func(int64, string) {})
			if mode == "bad-range" || mode == "corrupt" {
				if err == nil {
					t.Fatal("bad response accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			actual, err := os.ReadFile(name)
			if err != nil || string(actual) != body {
				t.Fatalf("invalid resumed file %q %v", actual, err)
			}
		})
	}
}
func TestCancellationAndAdmission(t *testing.T) {
	m, _ := fixture(t, HuggingFace, nil)
	p := prepare(t, m, HuggingFace)
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() }))
	defer server.Close()
	for i := range p.Files {
		p.Files[i].URL = server.URL
	}
	m.plans[p.ID] = p
	s, err := m.Start(context.Background(), p.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	if _, err := m.Start(context.Background(), p.ID, true); err == nil {
		t.Fatal("concurrent download accepted")
	}
	if err := m.Cancel(s.ID); err != nil {
		t.Fatal(err)
	}
	if result := waitDownload(t, m, s.ID); result.Status != "cancelled" {
		t.Fatalf("not cancelled: %+v", result)
	}
}
func TestPathsAndStorage(t *testing.T) {
	for _, name := range []string{"../outside", "/absolute", "a/../../b", `a\b`, "C:drive", "a/../b"} {
		if safePath(name) {
			t.Fatalf("unsafe path accepted: %s", name)
		}
	}
	root := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "link")); err == nil {
		if _, err := safeDestination(root, "link/model"); err == nil {
			t.Fatal("symlink accepted")
		}
	}
	storage, err := diskSpace(filepath.Join(root, "not-created", "models"))
	if err != nil || storage.TotalBytes == 0 || storage.Volume == "" {
		t.Fatalf("partition query failed: %+v %v", storage, err)
	}
}
func TestVariantsAndMoE(t *testing.T) {
	v := variants([]File{{Path: "weights-Q4-00001-of-00002.gguf", Size: 10}, {Path: "weights-Q4-00002-of-00002.gguf", Size: 20}, {Path: "mmproj.gguf", Size: 1}, {Path: "model-00001.safetensors", Size: 5}})
	if len(v) != 2 || v[1].Size != 30 || len(v[1].Files) != 2 {
		t.Fatalf("invalid shard grouping: %+v", v)
	}
	if architecture("qwen3_moe") != "MoE" || architecture("qwen3") != "Dense" || architecture("new_unknown") != "Unknown" {
		t.Fatal("architecture inference is incorrect")
	}
}

func TestSearchPaginationAndMetadata(t *testing.T) {
	for _, source := range []Source{HuggingFace, ModelScope} {
		t.Run(string(source), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/models/Qwen/test/treesize/revision" {
					fmt.Fprint(w, `{"path":"/","size":2500000000}`)
					return
				}
				if r.URL.Query().Get("search") != "Qwen" {
					t.Error("search query lost")
				}
				if source == HuggingFace {
					for _, field := range r.URL.Query()["expand[]"] {
						if field == "id" {
							t.Error("invalid expand field")
						}
					}
					w.Header().Set("Link", `<https://huggingface.co/api/models?cursor=next-page>; rel="next"`)
					fmt.Fprint(w, `[{"id":"Qwen/test","sha":"revision","config":{"model_type":"qwen3_moe"},"safetensors":{"total":1000000000},"downloads":123}]`)
				} else {
					if r.URL.Query().Get("page_number") != "1" {
						t.Error("invalid ModelScope page")
					}
					fmt.Fprint(w, `{"success":true,"data":{"models":[{"id":"Qwen/test","display_name":"Qwen","params":1000000000,"file_size":2500000000,"tags":["model_type:qwen3_moe"],"downloads":123}],"total_count":21}}`)
				}
			}))
			defer server.Close()
			c := NewCatalog()
			c.hf, c.ms = server.URL, server.URL
			page, err := c.Search(context.Background(), source, "Qwen", "")
			if err != nil || len(page.Models) != 1 || page.Next == "" {
				t.Fatalf("invalid page %+v %v", page, err)
			}
			if model := page.Models[0]; model.Parameters != 1000000000 || model.SizeBytes != 2500000000 || model.Architecture != "MoE" || model.Downloads != 123 {
				t.Fatalf("metadata lost %+v", model)
			}
		})
	}
}

func TestSearchKeepsModelsWhenSizeIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models":
			fmt.Fprint(w, `[{"id":"acme/private"},{"id":"acme/missing"},{"id":"acme/invalid"}]`)
		case "/api/models/acme/private/treesize/main":
			w.WriteHeader(http.StatusForbidden)
		case "/api/models/acme/invalid/treesize/main":
			fmt.Fprint(w, `{"size":-1}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()
	c := NewCatalog()
	c.hf = server.URL
	page, err := c.Search(context.Background(), HuggingFace, "", "")
	if err != nil || len(page.Models) != 3 {
		t.Fatalf("optional size metadata blocked search: %+v %v", page, err)
	}
	for _, model := range page.Models {
		if model.SizeBytes != 0 {
			t.Fatalf("missing size must remain unknown: %+v", model)
		}
	}
}

func TestRepositorySizeIncludesAllVariantsAndAncillaryFiles(t *testing.T) {
	for _, source := range []Source{HuggingFace, ModelScope} {
		t.Run(string(source), func(t *testing.T) {
			m, files := fixture(t, source, nil)
			files["alternate.gguf"] = "alternate-model-weights"
			details, err := m.Catalog.Inspect(context.Background(), source, "acme/model")
			if err != nil {
				t.Fatal(err)
			}
			var total int64
			for _, body := range files {
				total += int64(len(body))
			}
			if details.SizeBytes != total || len(details.Variants) != 2 || details.SizeBytes <= details.Variants[0].Size {
				t.Fatalf("repository size is not the full snapshot: %+v", details)
			}
		})
	}
}

func TestCancelSizeLookup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/models" {
			fmt.Fprint(w, `[{"id":"acme/model"}]`)
			return
		}
		cancel()
		<-r.Context().Done()
	}))
	defer server.Close()
	c := NewCatalog()
	c.hf = server.URL
	if _, err := c.Search(ctx, HuggingFace, "", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("size lookup ignored request cancellation: %v", err)
	}
}

func TestUnavailableStorageAndWorkerPreventAdding(t *testing.T) {
	m, _ := fixture(t, HuggingFace, nil)
	m.options.Workers = func() []domain.Worker { return nil }
	p := prepare(t, m, HuggingFace)
	if p.Supported || !strings.Contains(p.Reason, "worker") {
		t.Fatalf("worker compatibility not checked: %+v", p)
	}
	m.space = func(string) (Storage, error) { return Storage{}, errors.New("cannot query target partition") }
	if _, err := m.Prepare(context.Background(), PlanInput{Source: HuggingFace, Repository: "acme/model", Variant: "model-Q4.gguf"}); err == nil {
		t.Fatal("unknown disk space was treated as unlimited")
	}
}

func TestNestedQwen35Configuration(t *testing.T) {
	m, files := fixture(t, HuggingFace, nil)
	files["config.json"] = `{"model_type":"qwen3_5","text_config":{"model_type":"qwen3_5_text","max_position_embeddings":262144}}`
	p := prepare(t, m, HuggingFace)
	if !p.Supported || p.Local.OutputParser != "qwen3_5" || p.ContextWindow != 8192 {
		t.Fatalf("nested text config lost model family: %+v", p)
	}
}

func TestSafetensorsPlanKeepsTokenizerWithWeights(t *testing.T) {
	m, files := fixture(t, HuggingFace, nil)
	m.options.Workers = func() []domain.Worker {
		return []domain.Worker{{Backend: domain.MLX, Enabled: true, Command: "/mlx-worker"}}
	}
	files["model.safetensors"] = "tensor-contents"
	p, err := m.Prepare(context.Background(), PlanInput{Source: HuggingFace, Repository: "acme/model", Variant: "safetensors"})
	if err != nil {
		t.Fatal(err)
	}
	r := p.Local.Resources[0]
	if r.Backend != domain.MLX || r.Format != "safetensors" || r.WeightsPath != p.Directory || r.CodecDirectory != p.Directory {
		t.Fatalf("invalid MLX paths: %+v", r)
	}
	for _, file := range p.Files {
		if strings.HasSuffix(file.Path, ".gguf") {
			t.Fatal("unselected GGUF included in Safetensors download")
		}
	}
	if !p.Supported {
		if _, err := m.Start(context.Background(), p.ID, true); err == nil {
			t.Fatal("unsupported platform accepted Safetensors download")
		}
		return
	}
	s, err := m.Start(context.Background(), p.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if state := waitDownload(t, m, s.ID); state.Status != "downloaded" {
		t.Fatalf("Safetensors download failed: %+v", state)
	}
}

func TestAutomaticTokenizerResolution(t *testing.T) {
	for _, mode := range []string{"same-repository", "base-model", "missing", "base-unavailable"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				base := strings.Contains(r.URL.Path, "/acme/base")
				if base && mode == "base-unavailable" {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				switch {
				case r.URL.Path == "/api/models/acme/model":
					baseModel := "acme/base"
					if mode == "missing" {
						baseModel = ""
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"sha": "revision", "cardData": map[string]any{"base_model": baseModel}})
				case r.URL.Path == "/api/models/acme/base":
					if mode == "same-repository" {
						t.Error("repository already includes tokenizer; base lookup is unnecessary")
					}
					fmt.Fprint(w, `{"sha":"revision"}`)
				case strings.Contains(r.URL.Path, "/tree/"):
					files := []File{{Path: "config.json", Size: 100}}
					if !base {
						files = append(files, File{Path: "model.gguf", Size: 1000})
					}
					if base || mode == "same-repository" {
						files = append(files, File{Path: "tokenizer.json", Size: 200})
					}
					entries := make([]map[string]any, 0, len(files))
					for _, file := range files {
						entries = append(entries, map[string]any{"type": "file", "path": file.Path, "size": file.Size})
					}
					_ = json.NewEncoder(w).Encode(entries)
				default:
					fmt.Fprint(w, `{"model_type":"qwen3"}`)
				}
			}))
			defer server.Close()
			m := NewManager(context.Background(), Options{Directory: filepath.Join(t.TempDir(), "models"), Workers: func() []domain.Worker {
				return []domain.Worker{{Backend: domain.LlamaCpp, Enabled: true, Command: "/worker"}}
			}})
			defer m.cancel()
			m.Catalog.hf = server.URL
			plan, err := m.Prepare(context.Background(), PlanInput{Source: HuggingFace, Repository: "acme/model", Variant: "model.gguf"})
			if err != nil {
				t.Fatal(err)
			}
			supported := mode == "same-repository" || mode == "base-model"
			if plan.Supported != supported {
				t.Fatalf("automatic resolution failed: %+v", plan)
			}
			if mode == "base-model" && (plan.CodecRepository != "acme/base" || filepath.Base(plan.Local.Resources[0].CodecDirectory) != "codec") {
				t.Fatalf("base tokenizer was not selected: %+v", plan)
			}
			if !supported && (!strings.Contains(plan.Reason, "tokenizer.json") || strings.Contains(plan.Reason, "Enter")) {
				t.Fatalf("missing tokenizer warning is misleading: %+v", plan)
			}
		})
	}
}
