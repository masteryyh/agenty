package storage

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

var ErrProviderNotFound = catalog.ErrProviderNotFound

type CatalogRepository struct {
	providersDir     string
	builtinProviders map[shared.Code]*catalog.Provider
	builtinOrder     []shared.Code
	cacheMu          sync.RWMutex
}

type modelDiscoveryCache struct {
	ExpiresAt time.Time       `json:"expiresAt"`
	Models    []catalog.Model `json:"models"`
}

func NewCatalogRepository(providersDir string, builtinProviders ...*catalog.Provider) *CatalogRepository {
	builtins := make(map[shared.Code]*catalog.Provider, len(builtinProviders))
	order := make([]shared.Code, 0, len(builtinProviders))
	for _, provider := range builtinProviders {
		if provider == nil || !provider.Code.Valid() {
			continue
		}
		copy := cloneProvider(provider)
		copy.Builtin = true
		builtins[copy.Code] = copy
		order = append(order, copy.Code)
	}
	return &CatalogRepository{providersDir: providersDir, builtinProviders: builtins, builtinOrder: order}
}

func (r *CatalogRepository) Get(_ context.Context, code shared.Code) (*catalog.Provider, error) {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	return r.getLocked(code, true)
}

func (r *CatalogRepository) getLocked(code shared.Code, includeDiscoveryCache bool) (*catalog.Provider, error) {
	if builtin, ok := r.builtinProviders[code]; ok {
		provider := cloneProvider(builtin)
		apiKey, err := r.readAPIKey(code)
		if err != nil {
			return nil, err
		}
		provider.APIKey = apiKey
		if includeDiscoveryCache {
			r.applyModelDiscoveryCache(provider)
		}
		return provider, nil
	}

	providerPath := filepath.Join(r.providersDir, code.String()+".json")
	data, err := os.ReadFile(providerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrProviderNotFound
		}
		return nil, err
	}

	var provider catalog.Provider
	if err := json.Unmarshal(data, &provider); err != nil {
		return nil, err
	}
	provider.ModelsCached = false
	normalizeModels(&provider)
	if includeDiscoveryCache {
		r.applyModelDiscoveryCache(&provider)
	}

	return &provider, nil
}

// NeedsModelDiscovery reports whether a provider with no embedded/persisted
// models has no fresh discovery cache. Expired cache entries remain available
// through Get so callers can continue using the last known model metadata while
// a subsequent list operation refreshes it.
func (r *CatalogRepository) NeedsModelDiscovery(_ context.Context, code shared.Code) (bool, error) {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	provider, err := r.getLocked(code, false)
	if err != nil {
		return false, err
	}
	if len(provider.Models) > 0 {
		return false, nil
	}

	cache, err := r.readModelDiscoveryCache(code)
	if err != nil {
		return true, nil
	}
	return cache == nil || cache.ExpiresAt.IsZero() || !time.Now().UTC().Before(cache.ExpiresAt), nil
}

func (r *CatalogRepository) List(ctx context.Context) ([]*catalog.Provider, error) {
	providers := make([]*catalog.Provider, 0, len(r.builtinProviders))
	for _, code := range r.builtinOrder {
		provider, err := r.Get(ctx, code)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	entries, err := os.ReadDir(r.providersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return providers, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		code, err := shared.NewCode(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
		if err != nil {
			continue
		}
		if _, builtin := r.builtinProviders[code]; builtin {
			continue
		}

		provider, err := r.Get(ctx, code)
		if err != nil {
			if err == ErrProviderNotFound {
				continue
			}
			return nil, err
		}
		providers = append(providers, provider)
	}

	return providers, nil
}

func (r *CatalogRepository) Save(_ context.Context, provider *catalog.Provider) error {
	if provider == nil || !provider.Code.Valid() {
		return fmt.Errorf("storage: invalid provider code")
	}
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	if err := r.invalidateModelDiscoveryCacheLocked(provider.Code); err != nil {
		return err
	}
	if _, builtin := r.builtinProviders[provider.Code]; builtin {
		return r.saveAPIKey(provider.Code, provider.APIKey)
	}
	if err := os.MkdirAll(r.providersDir, 0700); err != nil {
		return err
	}

	// ModelsCached is a transient RPC hint, not part of provider config.
	provider.ModelsCached = false
	normalizeModels(provider)
	providerData, err := json.MarshalIndent(provider, "", "  ")
	if err != nil {
		return err
	}

	providerPath := filepath.Join(r.providersDir, provider.Code.String()+".json")
	return os.WriteFile(providerPath, providerData, 0600)
}

func (r *CatalogRepository) Delete(_ context.Context, code shared.Code) error {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	if _, builtin := r.builtinProviders[code]; builtin {
		return catalog.ErrBuiltinProviderReadOnly
	}
	providerPath := filepath.Join(r.providersDir, code.String()+".json")
	if err := os.Remove(providerPath); err != nil {
		if os.IsNotExist(err) {
			return ErrProviderNotFound
		}
		return err
	}
	if err := r.invalidateModelDiscoveryCacheLocked(code); err != nil {
		return err
	}
	return nil
}

// ReplaceModels stores the latest discovered model list without changing the
// provider's hand-authored configuration. The list becomes visible through
// Get/List until its expiration time.
func (r *CatalogRepository) ReplaceModels(
	_ context.Context,
	code shared.Code,
	models []catalog.Model,
	expiresAt time.Time,
) error {
	if !code.Valid() {
		return fmt.Errorf("storage: invalid provider code")
	}
	if expiresAt.IsZero() {
		return fmt.Errorf("storage: model discovery cache expiration is required")
	}

	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	if _, err := r.getLocked(code, false); err != nil {
		return err
	}

	normalized := slices.Clone(models)
	for index := range normalized {
		catalog.NormalizeReasoningCapabilities(&normalized[index])
		if normalized[index].MaxOutputTokens <= 0 {
			normalized[index].MaxOutputTokens = catalog.DefaultMaxOutputTokens
		}
	}
	if normalized == nil {
		normalized = make([]catalog.Model, 0)
	}

	data, err := json.MarshalIndent(modelDiscoveryCache{
		ExpiresAt: expiresAt.UTC(),
		Models:    normalized,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("storage: encode model discovery cache: %w", err)
	}
	cacheDir := filepath.Join(r.providersDir, ".models")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("storage: create model discovery cache directory: %w", err)
	}
	cachePath := filepath.Join(cacheDir, code.String()+".json")
	if err := os.WriteFile(cachePath, data, 0600); err != nil {
		return fmt.Errorf("storage: write model discovery cache: %w", err)
	}
	return nil
}

func (r *CatalogRepository) applyModelDiscoveryCache(provider *catalog.Provider) {
	if len(provider.Models) > 0 {
		return
	}
	cache, err := r.readModelDiscoveryCache(provider.Code)
	if err != nil {
		return
	}
	if cache == nil {
		return
	}
	provider.Models = cache.Models
	provider.ModelsCached = true
}

func (r *CatalogRepository) readModelDiscoveryCache(code shared.Code) (*modelDiscoveryCache, error) {
	data, err := os.ReadFile(r.modelDiscoveryCachePath(code))
	if err != nil {
		return nil, err
	}

	var cache modelDiscoveryCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	normalizeModelsForCache(&cache)
	return &cache, nil
}

func normalizeModelsForCache(cache *modelDiscoveryCache) {
	if cache.Models == nil {
		cache.Models = make([]catalog.Model, 0)
	}
	for index := range cache.Models {
		catalog.NormalizeReasoningCapabilities(&cache.Models[index])
		if cache.Models[index].MaxOutputTokens <= 0 {
			cache.Models[index].MaxOutputTokens = catalog.DefaultMaxOutputTokens
		}
	}
}

func (r *CatalogRepository) modelDiscoveryCachePath(code shared.Code) string {
	return filepath.Join(r.providersDir, ".models", code.String()+".json")
}

func (r *CatalogRepository) invalidateModelDiscoveryCacheLocked(code shared.Code) error {
	err := os.Remove(r.modelDiscoveryCachePath(code))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: remove model discovery cache: %w", err)
	}
	return nil
}

func normalizeModels(provider *catalog.Provider) {
	if provider.Models == nil {
		provider.Models = make([]catalog.Model, 0)
	}
	for index := range provider.Models {
		catalog.NormalizeReasoningCapabilities(&provider.Models[index])
		if provider.Models[index].MaxOutputTokens <= 0 {
			provider.Models[index].MaxOutputTokens = catalog.DefaultMaxOutputTokens
		}
	}
}

type providerCredentials struct {
	APIKey string `json:"apiKey"`
}

func (r *CatalogRepository) readAPIKey(code shared.Code) (string, error) {
	providerPath := filepath.Join(r.providersDir, code.String()+".json")
	data, err := os.ReadFile(providerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	var credentials providerCredentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		return "", err
	}
	return credentials.APIKey, nil
}

func (r *CatalogRepository) saveAPIKey(code shared.Code, apiKey string) error {
	if err := os.MkdirAll(r.providersDir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(providerCredentials{APIKey: apiKey}, "", "  ")
	if err != nil {
		return err
	}
	providerPath := filepath.Join(r.providersDir, code.String()+".json")
	return os.WriteFile(providerPath, data, 0600)
}

func cloneProvider(provider *catalog.Provider) *catalog.Provider {
	copy := *provider
	copy.Models = make([]catalog.Model, len(provider.Models))
	copy.Models = append(copy.Models[:0], provider.Models...)
	for index := range copy.Models {
		catalog.NormalizeReasoningCapabilities(&copy.Models[index])
	}
	copy.Metadata = maps.Clone(provider.Metadata)
	return &copy
}
