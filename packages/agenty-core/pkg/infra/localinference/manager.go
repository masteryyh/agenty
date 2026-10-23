package localinference

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	domain "github.com/masteryyh/agenty-core/pkg/domain/localinference"
	"github.com/masteryyh/agenty-core/pkg/infra/llmcodec"
	"github.com/masteryyh/agenty-core/pkg/infra/workeripc"
)

type LoadedModel struct {
	ModelHandle     string `json:"modelHandle"`
	ContextWindow   int    `json:"contextWindow"`
	VocabSize       int    `json:"vocabSize"`
	TokenizerSHA256 string `json:"tokenizerSha256"`
}

var ErrInvalidConfig = errors.New("invalid local inference configuration")

type Lease struct {
	Context  context.Context
	Client   *workeripc.Client
	Codec    *llmcodec.Model
	Model    LoadedModel
	Resource domain.Resource
}
type slot struct {
	ready    chan struct{}
	config   domain.Worker
	gate     chan struct{}
	queue    chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	client   *workeripc.Client
	codec    *llmcodec.Model
	resource domain.Resource
	model    LoadedModel
	state    domain.WorkerState
}
type Manager struct {
	ctx      context.Context
	configMu sync.Mutex
	mu       sync.Mutex
	enabled  bool
	closed   bool
	slots    map[string]*slot
	retired  map[*slot]struct{}
	publish  func(domain.Snapshot)
}

func New(ctx context.Context, publish func(domain.Snapshot)) *Manager {
	return &Manager{ctx: ctx, slots: make(map[string]*slot), retired: make(map[*slot]struct{}), publish: publish}
}

func (m *Manager) Snapshot() domain.Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	states := make([]domain.WorkerState, 0, len(m.slots))
	for _, s := range m.slots {
		states = append(states, s.state)
	}
	slices.SortFunc(states, func(a, b domain.WorkerState) int { return strings.Compare(a.ID, b.ID) })
	return domain.Snapshot{Enabled: m.enabled, Workers: states}
}
func (m *Manager) emit() {
	if m.publish != nil {
		m.publish(m.Snapshot())
	}
}
func (m *Manager) state(s *slot, status string, err error) {
	m.mu.Lock()
	s.state.Status = status
	s.state.Error = ""
	if err != nil {
		s.state.Error = err.Error()
	}
	m.mu.Unlock()
	m.emit()
}

func (m *Manager) Configure(cfg domain.Config) error {
	m.configMu.Lock()
	defer m.configMu.Unlock()
	if m.closed {
		return fmt.Errorf("local inference manager is shutting down")
	}
	return m.configure(cfg)
}

// UpdateConfig serializes persistence and runtime activation across API patches
// and completed installations. Events are published only after persistence.
func (m *Manager) UpdateConfig(store interface {
	LocalInference() domain.Config
	SetLocalInference(domain.Config) error
}, change func(*domain.Config)) (domain.Config, error) {
	m.configMu.Lock()
	defer m.configMu.Unlock()
	if m.closed {
		return domain.Config{}, fmt.Errorf("local inference manager is shutting down")
	}
	cfg := store.LocalInference()
	change(&cfg)
	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}
	if err := store.SetLocalInference(cfg); err != nil {
		return cfg, err
	}
	cfg = store.LocalInference()
	return cfg, m.configure(cfg)
}

func (m *Manager) configure(cfg domain.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	old := m.slots
	wasEnabled := m.enabled
	m.slots = make(map[string]*slot)
	m.enabled = cfg.Enabled
	for _, w := range cfg.Workers {
		previous := old[w.ID]
		if previous != nil && wasEnabled == cfg.Enabled && reflect.DeepEqual(previous.config, w) {
			m.slots[w.ID] = previous
			delete(old, w.ID)
			continue
		}
		ctx, cancel := context.WithCancel(m.ctx)
		s := &slot{ready: make(chan struct{}), config: w, gate: make(chan struct{}, 1), queue: make(chan struct{}, 32), ctx: ctx, cancel: cancel,
			state: domain.WorkerState{ID: w.ID, Backend: w.Backend, Status: "stopped"}}
		m.slots[w.ID] = s
		if previous != nil && cfg.Enabled && w.Enabled && wasEnabled && previous.config.Enabled {
			delete(old, w.ID)
			m.retired[previous] = struct{}{}
			// Keep the active call on its original version. The replacement only
			// becomes startable after that version is idle and fully released.
			go func() {
				previous.gate <- struct{}{}
				previous.cancel()
				m.cleanup(previous)
				<-previous.gate
				m.mu.Lock()
				delete(m.retired, previous)
				m.mu.Unlock()
				close(s.ready)
			}()
		} else {
			close(s.ready)
		}
	}
	for _, s := range old {
		m.retired[s] = struct{}{}
	}
	m.mu.Unlock()
	for _, s := range old {
		s.cancel()
		go func() {
			s.gate <- struct{}{}
			m.cleanup(s)
			<-s.gate
			m.mu.Lock()
			delete(m.retired, s)
			m.mu.Unlock()
		}()
	}
	m.emit()
	if cfg.Enabled {
		go m.warmup()
	}
	return nil
}

func (m *Manager) warmup() {
	m.mu.Lock()
	candidates := make([]*slot, 0)
	for _, s := range m.slots {
		if s.config.Enabled && (s.config.Backend != domain.MLX || runtime.GOOS == "darwin" && runtime.GOARCH == "arm64") {
			candidates = append(candidates, s)
		}
	}
	m.mu.Unlock()
	slices.SortStableFunc(candidates, func(a, b *slot) int {
		if a.config.Backend == domain.MLX && b.config.Backend != domain.MLX {
			return -1
		}
		if b.config.Backend == domain.MLX && a.config.Backend != domain.MLX {
			return 1
		}
		return strings.Compare(a.config.ID, b.config.ID)
	})
	for _, s := range candidates {
		select {
		case s.gate <- struct{}{}:
		case <-s.ctx.Done():
			return
		}
		err := m.start(s.ctx, s)
		<-s.gate
		if err == nil {
			return
		}
	}
}

func (m *Manager) start(ctx context.Context, s *slot) error {
	select {
	case <-s.ready:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if s.client != nil && s.client.Alive() {
		return nil
	}
	m.cleanup(s)
	m.state(s, "starting", nil)
	env := []string{}
	// Pass process essentials without forwarding provider credentials.
	for _, key := range []string{"PATH", "HOME", "TMPDIR", "TEMP", "TMP", "SystemRoot", "WINDIR", "LOCALAPPDATA"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	env = append(env, "PYTHONNOUSERSITE=1", "HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1")
	client, err := workeripc.Start(s.ctx, s.config.Command, s.config.Arguments, env)
	if err != nil {
		m.state(s, "failed", err)
		return err
	}
	s.client = client
	handshakeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var info workeripc.Initialization
	err = client.Call(handshakeCtx, "initialize", map[string]any{"protocolVersion": 1, "maxFrameBytes": workeripc.MaxFrameBytes}, &info, nil)
	if err == nil && (info.ProtocolVersion != 1 || info.Backend != string(s.config.Backend) || info.MaxConcurrentGenerations != 1 || info.MaxFrameBytes <= 0 || info.MaxInputTokens <= 0 || info.MaxOutputTokens <= 0) {
		err = errors.New("worker handshake does not match configured backend or protocol")
	}
	if err != nil {
		m.cleanup(s)
		m.state(s, "failed", err)
		return err
	}
	client.Limit = min(workeripc.MaxFrameBytes, info.MaxFrameBytes)
	client.MaxInputTokens, client.MaxOutputTokens = info.MaxInputTokens, info.MaxOutputTokens
	m.mu.Lock()
	s.state.EngineVersion = info.EngineVersion
	m.mu.Unlock()
	m.state(s, "ready", nil)
	go func() {
		select {
		case <-client.Exited():
		case <-s.ctx.Done():
			return
		}
		select {
		case s.gate <- struct{}{}:
		case <-s.ctx.Done():
			return
		}
		defer func() { <-s.gate }()
		if s.client == client {
			m.cleanup(s)
			m.state(s, "failed", client.Err())
		}
	}()
	return nil
}

func (m *Manager) cleanup(s *slot) {
	if s.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = s.client.Close(ctx)
		cancel()
		s.client = nil
	}
	if s.codec != nil {
		_ = s.codec.Close()
		s.codec = nil
	}
	s.model = LoadedModel{}
	s.resource = domain.Resource{}
	m.mu.Lock()
	s.state.ModelResourceID = ""
	m.mu.Unlock()
}

func TokenizerFingerprint(directory string) (string, error) {
	file, err := os.Open(filepath.Join(directory, "tokenizer.json"))
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() > 128<<20 {
		return "", fmt.Errorf("tokenizer exceeds 128 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, (128<<20)+1))
	if err != nil {
		return "", err
	}
	if len(data) > 128<<20 {
		return "", fmt.Errorf("tokenizer exceeds 128 MiB")
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (m *Manager) load(ctx context.Context, s *slot, r domain.Resource, probeOnly bool) error {
	if err := m.start(ctx, s); err != nil {
		return err
	}
	fingerprint, err := TokenizerFingerprint(r.CodecDirectory)
	if err != nil {
		return err
	}
	if r.TokenizerSHA256 != "" && fingerprint != r.TokenizerSHA256 {
		return fmt.Errorf("tokenizer fingerprint changed for %s", r.ID)
	}
	r.TokenizerSHA256 = fingerprint
	if s.codec != nil && s.resource == r {
		return nil
	}
	opts := llmcodec.LoadOptions{IgnoreGGUF: r.Backend != domain.LlamaCpp}
	if r.Backend == domain.LlamaCpp {
		opts.GGUFMetadataPath = r.WeightsPath
	}
	codec, err := llmcodec.Load(r.CodecDirectory, opts)
	if err != nil {
		return err
	}
	if len(codec.Metadata.StopTokenIDs) == 0 {
		_ = codec.Close()
		return fmt.Errorf("local model requires validated stopping token IDs")
	}
	loaded := false
	defer func() {
		if !loaded {
			_ = codec.Close()
		}
	}()
	var probe struct {
		Compatible bool   `json:"compatible"`
		Reason     string `json:"reason"`
	}
	if err := s.client.Call(ctx, "probeModel", r, &probe, nil); err != nil {
		return err
	}
	if !probe.Compatible {
		return fmt.Errorf("resource %s incompatible: %s", r.ID, probe.Reason)
	}
	if probeOnly {
		return nil
	}
	if s.model.ModelHandle != "" {
		if err := s.client.Call(ctx, "unloadModel", map[string]string{"modelHandle": s.model.ModelHandle}, nil, nil); err != nil {
			return err
		}
		_ = s.codec.Close()
		s.codec = nil
		s.model = LoadedModel{}
	}
	m.state(s, "loading", nil)
	var model LoadedModel
	if err := s.client.Call(ctx, "loadModel", r, &model, nil); err != nil {
		return err
	}
	if model.ModelHandle == "" || model.ContextWindow <= 0 || model.VocabSize < codec.Metadata.VocabSize || model.TokenizerSHA256 != fingerprint {
		return fmt.Errorf("worker model identity, vocabulary or context limit mismatch")
	}
	s.codec, s.resource, s.model = codec, r, model
	loaded = true
	m.mu.Lock()
	s.state.ModelResourceID = r.ID
	m.mu.Unlock()
	m.state(s, "ready", nil)
	return nil
}

func (m *Manager) WithModel(ctx context.Context, model *domain.Model, call func(Lease) error) error {
	return m.withModel(ctx, model, call, false)
}

func (m *Manager) Probe(ctx context.Context, model *domain.Model) (domain.Resource, error) {
	if err := model.Validate(); err != nil {
		return domain.Resource{}, err
	}
	m.configMu.Lock()
	closed := m.closed
	m.mu.Lock()
	enabled := m.enabled
	workers := make([]domain.Worker, 0, len(m.slots))
	for _, s := range m.slots {
		if slices.ContainsFunc(model.Resources, func(r domain.Resource) bool { return r.Backend == s.config.Backend }) {
			workers = append(workers, s.config)
		}
	}
	m.mu.Unlock()
	m.configMu.Unlock()
	if closed {
		return domain.Resource{}, fmt.Errorf("local inference manager is shutting down")
	}
	if !enabled {
		// Setup can check installed engines before enabling background inference.
		// These processes are owned by this explicit probe and always reaped.
		probeCtx, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(m.ctx, cancel)
		temporary := New(probeCtx, nil)
		defer func() {
			cancel()
			stop()
			shutdownCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
			defer done()
			_ = temporary.Shutdown(shutdownCtx)
		}()
		if err := temporary.Configure(domain.Config{Enabled: true, Workers: workers}); err != nil {
			return domain.Resource{}, err
		}
		return temporary.Probe(probeCtx, model)
	}
	var resource domain.Resource
	err := m.withModel(ctx, model, func(lease Lease) error { resource = lease.Resource; return nil }, true)
	return resource, err
}

func (m *Manager) withModel(ctx context.Context, model *domain.Model, call func(Lease) error, probeOnly bool) error {
	if err := model.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	if !m.enabled {
		m.mu.Unlock()
		return fmt.Errorf("local_inference_disabled: enable local inference first")
	}
	slots := make([]*slot, 0, len(m.slots))
	for _, s := range m.slots {
		if s.config.Enabled {
			slots = append(slots, s)
		}
	}
	m.mu.Unlock()
	resources := slices.Clone(model.Resources)
	if model.BackendPreference == "auto" || model.BackendPreference == "" {
		slices.SortStableFunc(resources, func(a, b domain.Resource) int {
			if a.Backend == domain.MLX && b.Backend != domain.MLX {
				return -1
			}
			if b.Backend == domain.MLX && a.Backend != domain.MLX {
				return 1
			}
			return 0
		})
	}
	var failures []error
	for _, r := range resources {
		if model.BackendPreference != "" && model.BackendPreference != "auto" && string(r.Backend) != model.BackendPreference {
			continue
		}
		if r.Backend == domain.MLX && (runtime.GOOS != "darwin" || runtime.GOARCH != "arm64") {
			continue
		}
		for _, s := range slots {
			if s.config.Backend != r.Backend {
				continue
			}
			select {
			case s.queue <- struct{}{}:
			default:
				return fmt.Errorf("local inference queue is full")
			}
			select {
			case s.gate <- struct{}{}:
			case <-ctx.Done():
				<-s.queue
				return ctx.Err()
			case <-s.ctx.Done():
				<-s.queue
				continue
			}
			callCtx, cancel := context.WithCancel(ctx)
			stop := context.AfterFunc(s.ctx, cancel)
			err := m.load(callCtx, s, r, probeOnly)
			if err == nil {
				if !probeOnly {
					m.state(s, "generating", nil)
				}
				err = call(Lease{Context: callCtx, Client: s.client, Codec: s.codec, Model: s.model, Resource: r})
				if err != nil {
					m.state(s, "failed", err)
				} else {
					m.state(s, "ready", nil)
				}
				stop()
				cancel()
				<-s.gate
				<-s.queue
				return err // An accepted generation is never replayed on another backend.
			}
			m.cleanup(s)
			m.state(s, "failed", err)
			stop()
			cancel()
			<-s.gate
			<-s.queue
			failures = append(failures, fmt.Errorf("%s/%s: %w", s.config.ID, r.ID, err))
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
	}
	if len(failures) == 0 {
		return fmt.Errorf("no installed enabled worker matches this model's resources")
	}
	return errors.Join(failures...)
}

func (m *Manager) Shutdown(ctx context.Context) error {
	m.configMu.Lock()
	defer m.configMu.Unlock()
	m.closed = true
	m.mu.Lock()
	m.enabled = false
	slots := make([]*slot, 0, len(m.slots))
	for _, s := range m.slots {
		s.cancel()
		slots = append(slots, s)
	}
	for s := range m.retired {
		s.cancel()
		slots = append(slots, s)
	}
	m.mu.Unlock()
	for _, s := range slots {
		select {
		case s.gate <- struct{}{}:
			m.cleanup(s)
			<-s.gate
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
