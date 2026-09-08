package inspection

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var BuildVersion = "dev"

const MaxTranscriptBytes int64 = 64 << 20
const cacheBudget int64 = 128 << 20

var (
	ErrNotFound        = errors.New("session not found")
	ErrSnapshotExpired = errors.New("snapshot expired; refresh the session")
	ErrTooLarge        = errors.New("transcript exceeds the 64 MiB snapshot limit")
	ErrChanged         = errors.New("transcript changed while reading; refresh to retry")
)

type System struct {
	Agents             []string `json:"agents"`
	Models             []string `json:"models"`
	DataDir            string   `json:"dataDir"`
	SessionsDir        string   `json:"sessionsDir"`
	ReadOnly           bool     `json:"readOnly"`
	Scanning           bool     `json:"scanning"`
	Error              string   `json:"error,omitempty"`
	Generation         uint64   `json:"generation"`
	SessionCount       int      `json:"sessionCount"`
	MaxTranscriptBytes int64    `json:"maxTranscriptBytes"`
	Version            string   `json:"version"`
}

type catalogItem struct {
	entry SessionEntry
	info  fs.FileInfo
}

type cached struct {
	id       string
	snapshot *Snapshot
	cost     int64
}

type Store struct {
	dataDir     string
	sessionsDir string
	mu          sync.Mutex
	scanMu      sync.Mutex
	loadMu      sync.Mutex
	catalog     map[string]catalogItem
	cache       []cached
	system      System
}

func NewStore(dataDir string) *Store {
	return &Store{
		dataDir:     dataDir,
		sessionsDir: filepath.Join(dataDir, "sessions"),
		catalog:     map[string]catalogItem{},
		cache:       []cached{},
		system: System{
			DataDir:            dataDir,
			SessionsDir:        filepath.Join(dataDir, "sessions"),
			ReadOnly:           true,
			Scanning:           true,
			Agents:             []string{},
			Models:             []string{},
			MaxTranscriptBytes: MaxTranscriptBytes,
			Version:            BuildVersion,
		},
	}
}

func (s *Store) System() System {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.system
}

func (s *Store) Run(ctx context.Context) {
	s.Scan(ctx)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Scan(ctx)
		}
	}
}

// Scan publishes a complete directory generation atomically. OpenRoot keeps
// transcript paths inside the configured sessions directory, including symlinks.
func (s *Store) Scan(ctx context.Context) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	s.mu.Lock()
	previous := s.catalog
	s.mu.Unlock()
	catalog := map[string]catalogItem{}
	root, err := os.OpenRoot(s.sessionsDir)
	if err == nil {
		defer root.Close()
		err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".jsonl") {
				return nil
			}
			info, statErr := root.Stat(path)
			if statErr != nil || !info.Mode().IsRegular() {
				return nil
			}
			digest := sha256.Sum256([]byte(path))
			id := fmt.Sprintf("%x", digest[:16])
			old, exists := previous[id]
			if exists && os.SameFile(old.info, info) && old.info.Size() == info.Size() && old.info.ModTime() == info.ModTime() {
				catalog[id] = old
				return nil
			}
			item := SessionEntry{ID: id, SessionID: strings.TrimSuffix(filepath.Base(path), ".jsonl"),
				Path: path, UpdatedAt: info.ModTime(), Size: info.Size()}
			s.loadMu.Lock()
			raw, readErr := readFile(ctx, root, path)
			if readErr != nil {
				item.Error = readErr.Error()
			} else {
				hash := sha256.Sum256(raw)
				item.Revision = fmt.Sprintf("%x", hash[:16])
				snapshot, buildErr := Build(ctx, item, raw)
				if buildErr != nil {
					item.Error = buildErr.Error()
				} else {
					if snapshot.Detail.Title != nil {
						item.Title = *snapshot.Detail.Title
					}
					item.Agent = snapshot.Detail.AgentCode.String()
					if model := snapshot.Detail.CurrentModel; model != nil {
						item.Model = model.ProviderCode.String() + " / " + model.ModelCode.String()
					}
					for _, issue := range snapshot.Detail.Diagnostics {
						if issue.Severity != "info" {
							item.IssueCount++
						}
					}
				}
			}
			s.loadMu.Unlock()
			catalog[id] = catalogItem{entry: item, info: info}
			return nil
		})
	}
	if ctx.Err() != nil {
		return
	}
	errorMessage := ""
	if err != nil {
		errorMessage = err.Error()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := len(catalog) != len(s.catalog) || errorMessage != s.system.Error || s.system.Scanning
	for id, item := range catalog {
		if old, exists := s.catalog[id]; !exists || old.entry != item.entry {
			changed = true
			break
		}
	}
	agents, models := map[string]bool{}, map[string]bool{}
	for _, item := range catalog {
		if item.entry.Agent != "" {
			agents[item.entry.Agent] = true
		}
		if item.entry.Model != "" {
			models[item.entry.Model] = true
		}
	}
	s.system.Agents, s.system.Models = []string{}, []string{}
	for value := range agents {
		s.system.Agents = append(s.system.Agents, value)
	}
	for value := range models {
		s.system.Models = append(s.system.Models, value)
	}
	sort.Strings(s.system.Agents)
	sort.Strings(s.system.Models)
	s.catalog = catalog
	s.system.Scanning, s.system.Error, s.system.SessionCount = false, errorMessage, len(catalog)
	if changed {
		s.system.Generation++
	}
}

func readFile(ctx context.Context, root *os.Root, path string) ([]byte, error) {
	file, err := root.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, errors.New("transcript is not a regular file")
	}
	if before.Size() > MaxTranscriptBytes {
		return nil, ErrTooLarge
	}
	raw := make([]byte, 0, before.Size())
	buffer := make([]byte, 64<<10)
	remaining := before.Size()
	for remaining > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		count, readErr := file.Read(buffer[:min(int64(len(buffer)), remaining)])
		raw = append(raw, buffer[:count]...)
		remaining -= int64(count)
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil, ErrChanged
			}
			return nil, readErr
		}
	}
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	// Ordinary append is allowed: this snapshot ends at the initial file size.
	if after.Size() < before.Size() || (after.Size() == before.Size() && after.ModTime() != before.ModTime()) {
		return nil, ErrChanged
	}
	return raw, nil
}

func (s *Store) List(query, agent, model, since, until string, issues bool) []SessionEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := []SessionEntry{}
	query = strings.ToLower(query)
	for _, item := range s.catalog {
		e := item.entry
		date := e.UpdatedAt.Local().Format("2006-01-02")
		if query != "" && !strings.Contains(strings.ToLower(e.Title+" "+e.SessionID+" "+e.Path), query) {
			continue
		}
		if agent != "" && e.Agent != agent {
			continue
		}
		if model != "" && e.Model != model {
			continue
		}
		if (since != "" && date < since) || (until != "" && date > until) {
			continue
		}
		if issues && e.Error == "" && e.IssueCount == 0 {
			continue
		}
		items = append(items, e)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items
}

func (s *Store) Snapshot(ctx context.Context, id, revision string) (*Snapshot, error) {
	// Serialize cache misses to bound transient parsing memory.
	s.loadMu.Lock()
	defer s.loadMu.Unlock()
	s.mu.Lock()
	item, exists := s.catalog[id]
	if !exists {
		s.mu.Unlock()
		return nil, ErrNotFound
	}
	expected := revision
	if expected == "" {
		expected = item.entry.Revision
	}
	for i, c := range s.cache {
		if c.id == id && c.snapshot.Detail.Revision == expected {
			s.cache = append(append(s.cache[:i], s.cache[i+1:]...), c)
			s.mu.Unlock()
			return c.snapshot, nil
		}
	}
	s.mu.Unlock()
	root, err := os.OpenRoot(s.sessionsDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	raw, err := readFile(ctx, root, item.entry.Path)
	if err != nil {
		return nil, err
	}
	snapshot, err := Build(ctx, item.entry, raw)
	if err != nil {
		return nil, err
	}
	if revision != "" && snapshot.Detail.Revision != revision {
		return nil, ErrSnapshotExpired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Account for raw text and decoded objects; very large snapshots are served
	// without caching, so later reads require the source revision to still exist.
	cost := int64(len(raw))*4 + int64(len(snapshot.Records))*512
	if cost <= cacheBudget {
		total := cost
		for _, c := range s.cache {
			total += c.cost
		}
		for len(s.cache) > 0 && (total > cacheBudget || len(s.cache) >= 8) {
			total -= s.cache[0].cost
			s.cache = s.cache[1:]
		}
		s.cache = append(s.cache, cached{id: id, snapshot: snapshot, cost: cost})
	}
	return snapshot, nil
}
