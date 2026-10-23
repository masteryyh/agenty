// Package modelhub browses public model hubs and downloads immutable model resources.
package modelhub

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	json "github.com/bytedance/sonic"
)

type Source string

const (
	HuggingFace Source = "huggingface"
	ModelScope  Source = "modelscope"
)

type Model struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Parameters   int64    `json:"parameters,omitempty"`
	SizeBytes    int64    `json:"sizeBytes,omitempty"`
	Architecture string   `json:"architecture"`
	ModelType    string   `json:"modelType,omitempty"`
	Downloads    int64    `json:"downloads"`
	Tags         []string `json:"tags,omitempty"`
}
type Page struct {
	Models []Model `json:"models"`
	Next   string  `json:"next,omitempty"`
}
type File struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
	OID    string `json:"-"`
	URL    string `json:"-"`
}
type Variant struct {
	ID     string `json:"id"`
	Format string `json:"format"`
	Size   int64  `json:"size"`
	Files  []File `json:"-"`
}
type Details struct {
	Model
	Source    Source      `json:"source"`
	Revision  string      `json:"revision"`
	Variants  []Variant   `json:"variants"`
	BaseModel string      `json:"baseModel,omitempty"`
	Files     []File      `json:"-"`
	Config    modelConfig `json:"-"`
}
type modelConfig struct {
	ModelType    string       `json:"model_type"`
	Experts      int          `json:"num_experts"`
	LocalExperts int          `json:"num_local_experts"`
	Context      int          `json:"max_position_embeddings"`
	Text         *modelConfig `json:"text_config"`
}

func (c modelConfig) text() modelConfig {
	if c.Text != nil {
		return *c.Text
	}
	return c
}

func (c modelConfig) family() string {
	if c.ModelType != "" {
		return c.ModelType
	}
	return c.text().ModelType
}

type Catalog struct {
	client *http.Client
	hf     string
	ms     string
}

func NewCatalog() *Catalog {
	return &Catalog{client: &http.Client{Timeout: 30 * time.Second}, hf: "https://huggingface.co", ms: "https://modelscope.cn"}
}
func (c *Catalog) endpoint(source Source) (string, error) {
	switch source {
	case HuggingFace:
		return c.hf, nil
	case ModelScope:
		return c.ms, nil
	default:
		return "", fmt.Errorf("unknown model source %q", source)
	}
}
func validRepo(repo string) bool {
	parts := strings.Split(repo, "/")
	return len(parts) == 2 && slices.IndexFunc(parts, func(s string) bool {
		return s == "" || s == "." || s == ".." || strings.ContainsAny(s, "\\?#%\x00\r\n ")
	}) < 0
}
func (c *Catalog) get(ctx context.Context, address string, target any) (http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Agenty-model-hub")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch model metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("model hub returned HTTP %d: this private or gated model requires authorization", resp.StatusCode)
		}
		return nil, fmt.Errorf("model hub returned HTTP %d for %s", resp.StatusCode, req.URL.Path)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 16<<20 {
		return nil, fmt.Errorf("model metadata exceeds 16 MiB")
	}
	if err := json.Unmarshal(body, target); err != nil {
		return nil, fmt.Errorf("decode model metadata: %w", err)
	}
	return resp.Header, nil
}
func modelType(tags []string) string {
	for _, tag := range tags {
		if t, ok := strings.CutPrefix(tag, "model_type:"); ok {
			return t
		}
		if strings.HasPrefix(tag, "qwen3") || tag == "llama" || tag == "mistral" {
			return tag
		}
	}
	return ""
}
func architecture(t string) string {
	if strings.Contains(t, "moe") || strings.Contains(t, "mixtral") {
		return "MoE"
	}
	switch t {
	case "qwen3", "qwen3_5", "llama", "mistral", "gemma", "gemma2":
		return "Dense"
	}
	return "Unknown"
}
func (c *Catalog) Search(ctx context.Context, source Source, query, cursor string) (Page, error) {
	endpoint, err := c.endpoint(source)
	if err != nil {
		return Page{}, err
	}
	result := Page{Models: []Model{}}
	if source == HuggingFace {
		u, _ := url.Parse(endpoint + "/api/models")
		q := url.Values{"search": {query}, "sort": {"downloads"}, "direction": {"-1"}, "limit": {"20"}, "expand[]": {"safetensors", "config", "tags", "downloads", "sha"}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		u.RawQuery = q.Encode()
		var data []struct {
			ID          string      `json:"id"`
			SHA         string      `json:"sha"`
			Downloads   int64       `json:"downloads"`
			Tags        []string    `json:"tags"`
			Config      modelConfig `json:"config"`
			Safetensors struct {
				Total int64 `json:"total"`
			} `json:"safetensors"`
		}
		headers, err := c.get(ctx, u.String(), &data)
		if err != nil {
			return result, err
		}
		revisions := make(map[string]string, len(data))
		for _, item := range data {
			revisions[item.ID] = item.SHA
			t := item.Config.family()
			if t == "" {
				t = modelType(item.Tags)
			}
			result.Models = append(result.Models, Model{ID: item.ID, Name: item.ID, Parameters: item.Safetensors.Total, ModelType: t, Architecture: architecture(t), Tags: item.Tags, Downloads: item.Downloads})
		}
		for _, link := range strings.Split(headers.Get("Link"), ",") {
			if strings.Contains(link, `rel="next"`) {
				address := strings.Trim(strings.TrimSpace(strings.Split(link, ";")[0]), "<>")
				if next, err := url.Parse(address); err == nil {
					result.Next = next.Query().Get("cursor")
				}
			}
		}
		c.huggingFaceSizes(ctx, result.Models, revisions)
		return result, ctx.Err()
	}
	page := 1
	if cursor != "" {
		page, err = strconv.Atoi(cursor)
		if err != nil || page < 1 || page > 150 {
			return result, fmt.Errorf("invalid ModelScope page")
		}
	}
	u, _ := url.Parse(endpoint + "/openapi/v1/models")
	u.RawQuery = url.Values{"search": {query}, "sort": {"downloads"}, "page_size": {"20"}, "page_number": {strconv.Itoa(page)}}.Encode()
	var data struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Models []struct {
				ID        string   `json:"id"`
				Name      string   `json:"display_name"`
				Params    int64    `json:"params"`
				Size      int64    `json:"file_size"`
				Downloads int64    `json:"downloads"`
				Tags      []string `json:"tags"`
			} `json:"models"`
			Total int `json:"total_count"`
		} `json:"data"`
	}
	if _, err := c.get(ctx, u.String(), &data); err != nil {
		return result, err
	}
	if !data.Success {
		return result, fmt.Errorf("ModelScope search: %s", data.Message)
	}
	for _, item := range data.Data.Models {
		t := modelType(item.Tags)
		result.Models = append(result.Models, Model{ID: item.ID, Name: item.Name, Parameters: item.Params, SizeBytes: max(item.Size, 0), ModelType: t, Architecture: architecture(t), Tags: item.Tags, Downloads: item.Downloads})
	}
	if page*20 < min(data.Data.Total, 3000) {
		result.Next = strconv.Itoa(page + 1)
	}
	return result, nil
}

// Tree size includes every current repository file, including alternate quantizations.
// Optional metadata must not prevent browsing when a repository is inaccessible.
func (c *Catalog) huggingFaceSizes(ctx context.Context, models []Model, revisions map[string]string) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	jobs := make(chan int, len(models))
	for i := range models {
		jobs <- i
	}
	close(jobs)
	var workers sync.WaitGroup
	for range min(4, len(models)) {
		workers.Go(func() {
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				if !validRepo(models[i].ID) {
					continue
				}
				var data struct {
					Size int64 `json:"size"`
				}
				revision := revisions[models[i].ID]
				if revision == "" {
					revision = "main"
				}
				if _, err := c.get(ctx, c.hf+"/api/models/"+models[i].ID+"/treesize/"+url.PathEscape(revision), &data); err == nil {
					models[i].SizeBytes = max(data.Size, 0)
				}
			}
		})
	}
	workers.Wait()
}
func baseModel(value any) string {
	switch v := value.(type) {
	case string:
		if validRepo(v) {
			return v
		}
	case []any:
		if len(v) > 0 {
			return baseModel(v[0])
		}
	}
	return ""
}
func (c *Catalog) fileURL(source Source, repo, revision, filename string) string {
	endpoint, _ := c.endpoint(source)
	if source == HuggingFace {
		return endpoint + "/" + repo + "/resolve/" + url.PathEscape(revision) + "/" + strings.Join(escapeParts(filename), "/")
	}
	return endpoint + "/api/v1/models/" + repo + "/repo?" + url.Values{"Revision": {revision}, "FilePath": {filename}}.Encode()
}
func escapeParts(filename string) []string {
	parts := strings.Split(filename, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return parts
}
func (c *Catalog) Inspect(ctx context.Context, source Source, repo string) (Details, error) {
	endpoint, err := c.endpoint(source)
	if err != nil {
		return Details{}, err
	}
	if !validRepo(repo) {
		return Details{}, fmt.Errorf("model ID must be owner/repository")
	}
	d := Details{Model: Model{ID: repo, Name: repo, Architecture: "Unknown"}, Source: source, Variants: []Variant{}}
	if source == HuggingFace {
		var data struct {
			SHA         string         `json:"sha"`
			Config      modelConfig    `json:"config"`
			Card        map[string]any `json:"cardData"`
			Safetensors struct {
				Total int64 `json:"total"`
			} `json:"safetensors"`
		}
		if _, err := c.get(ctx, endpoint+"/api/models/"+repo, &data); err != nil {
			return d, err
		}
		d.Revision, d.Config, d.Parameters, d.BaseModel = data.SHA, data.Config, data.Safetensors.Total, baseModel(data.Card["base_model"])
		if d.Revision == "" {
			return d, fmt.Errorf("model hub omitted the repository revision")
		}
		cursor := ""
		for pages := 0; ; pages++ {
			if pages >= 100 {
				return d, fmt.Errorf("repository file listing exceeds 100 pages")
			}
			u, _ := url.Parse(endpoint + "/api/models/" + repo + "/tree/" + url.PathEscape(d.Revision))
			q := url.Values{"recursive": {"true"}, "limit": {"1000"}}
			if cursor != "" {
				q.Set("cursor", cursor)
			}
			u.RawQuery = q.Encode()
			var files []struct {
				Type string `json:"type"`
				Path string `json:"path"`
				Size int64  `json:"size"`
				OID  string `json:"oid"`
				LFS  *struct {
					OID string `json:"oid"`
				} `json:"lfs"`
			}
			headers, err := c.get(ctx, u.String(), &files)
			if err != nil {
				return d, err
			}
			for _, f := range files {
				if f.Type != "file" {
					continue
				}
				file := File{Path: f.Path, Size: f.Size, OID: f.OID, URL: c.fileURL(source, repo, d.Revision, f.Path)}
				if f.LFS != nil {
					file.SHA256 = f.LFS.OID
					file.OID = ""
				}
				d.Files = append(d.Files, file)
			}
			cursor = ""
			for _, link := range strings.Split(headers.Get("Link"), ",") {
				if strings.Contains(link, `rel="next"`) {
					next, e := url.Parse(strings.Trim(strings.TrimSpace(strings.Split(link, ";")[0]), "<>"))
					if e == nil {
						cursor = next.Query().Get("cursor")
					}
				}
			}
			if cursor == "" {
				break
			}
		}
	} else {
		var info struct {
			Code    int    `json:"Code"`
			Message string `json:"Message"`
			Data    struct {
				Base []string `json:"BaseModel"`
				Name string   `json:"ChineseName"`
				Tags []string `json:"Tags"`
			} `json:"Data"`
		}
		if _, err := c.get(ctx, endpoint+"/api/v1/models/"+repo, &info); err != nil {
			return d, err
		}
		if info.Code != 200 {
			return d, fmt.Errorf("ModelScope model: %s", info.Message)
		}
		if len(info.Data.Base) > 0 {
			d.BaseModel = info.Data.Base[0]
		}
		var data struct {
			Code    int    `json:"Code"`
			Message string `json:"Message"`
			Data    struct {
				Files []struct {
					Path     string `json:"Path"`
					Size     int64  `json:"Size"`
					SHA      string `json:"Sha256"`
					Revision string `json:"Revision"`
					Type     string `json:"Type"`
				} `json:"Files"`
			} `json:"Data"`
		}
		if _, err := c.get(ctx, endpoint+"/api/v1/models/"+repo+"/repo/files?Revision=master&Recursive=true", &data); err != nil {
			return d, err
		}
		if data.Code != 200 {
			return d, fmt.Errorf("ModelScope files: %s", data.Message)
		}
		for _, f := range data.Data.Files {
			if f.Type != "blob" {
				continue
			}
			if f.Revision == "" {
				return d, fmt.Errorf("ModelScope omitted file revision")
			}
			d.Files = append(d.Files, File{Path: f.Path, Size: f.Size, SHA256: f.SHA, URL: c.fileURL(source, repo, f.Revision, f.Path)})
		}
		d.Revision = "file-revisions"
	}
	if config, ok := findFile(d.Files, "config.json"); ok {
		if _, err := c.get(ctx, config.URL, &d.Config); err != nil {
			return d, fmt.Errorf("read model config: %w", err)
		}
	}
	d.ModelType = d.Config.family()
	d.Architecture = architecture(d.ModelType)
	if d.Config.text().Experts > 0 || d.Config.text().LocalExperts > 0 {
		d.Architecture = "MoE"
	}
	d.Variants = variants(d.Files)
	for _, file := range d.Files {
		if file.Size < 0 || file.Size > (1<<63-1)-d.SizeBytes {
			d.SizeBytes = 0
			break
		}
		d.SizeBytes += file.Size
	}
	return d, nil
}
func findFile(files []File, name string) (File, bool) {
	for _, f := range files {
		if f.Path == name {
			return f, true
		}
	}
	return File{}, false
}

var splitGGUF = regexp.MustCompile(`^(.*)-[0-9]{5}-of-[0-9]{5}\.gguf$`)

func variants(files []File) []Variant {
	groups := map[string]*Variant{}
	for _, f := range files {
		var id, format string
		switch {
		case strings.HasSuffix(f.Path, ".gguf") && !strings.Contains(strings.ToLower(f.Path), "mmproj"):
			id, format = f.Path, "gguf"
			if match := splitGGUF.FindStringSubmatch(id); match != nil {
				id = match[1] + ".gguf"
			}
		case path.Dir(f.Path) == "." && strings.HasPrefix(f.Path, "model") && strings.HasSuffix(f.Path, ".safetensors"):
			id, format = "safetensors", "safetensors"
		default:
			continue
		}
		if groups[id] == nil {
			groups[id] = &Variant{ID: id, Format: format}
		}
		groups[id].Files = append(groups[id].Files, f)
		groups[id].Size += f.Size
	}
	result := make([]Variant, 0, len(groups))
	for _, v := range groups {
		slices.SortFunc(v.Files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
		result = append(result, *v)
	}
	slices.SortFunc(result, func(a, b Variant) int { return strings.Compare(a.ID, b.ID) })
	return result
}
