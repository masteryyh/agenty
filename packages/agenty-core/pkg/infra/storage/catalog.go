package storage

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
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
	modelCache       map[shared.Code]modelDiscoveryCache
}

type modelDiscoveryCache struct {
	ExpiresAt time.Time
	Models    []catalog.Model
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
	return &CatalogRepository{
		providersDir:     providersDir,
		builtinProviders: builtins,
		builtinOrder:     order,
		modelCache:       make(map[shared.Code]modelDiscoveryCache),
	}
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

// NeedsModelDiscovery reports whether a provider has no fresh discovery cache
// or has no configured models. Expired cache entries remain available through
// Get so callers can continue using the last known model metadata while a
// subsequent list operation refreshes it.
func (r *CatalogRepository) NeedsModelDiscovery(_ context.Context, code shared.Code) (bool, error) {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	provider, err := r.getLocked(code, false)
	if err != nil {
		return false, err
	}
	cache, ok := r.modelCache[code]
	if ok {
		return cache.ExpiresAt.IsZero() || !time.Now().UTC().Before(cache.ExpiresAt), nil
	}
	return len(provider.Models) == 0, nil
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
	if _, builtin := r.builtinProviders[provider.Code]; builtin {
		delete(r.modelCache, provider.Code)
		return r.saveAPIKey(provider.Code, provider.APIKey)
	}
	if err := os.MkdirAll(r.providersDir, 0700); err != nil {
		return err
	}

	providerToPersist := cloneProvider(provider)
	if provider.ModelsCached {
		configured, err := r.getLocked(provider.Code, false)
		if err != nil {
			return err
		}
		cache, hasCache := r.modelCache[provider.Code]
		if hasCache && providerConfigurationEqual(provider, configured) {
			cache.Models = retainedCachedModels(cache.Models, provider.Models)
			r.modelCache[provider.Code] = cache
			providerToPersist.Models = mergeConfiguredAndEditedModels(
				configured.Models,
				provider.Models,
				cache.Models,
			)
		} else {
			providerToPersist.Models = configured.Models
			delete(r.modelCache, provider.Code)
		}
	} else {
		delete(r.modelCache, provider.Code)
	}
	providerToPersist.ModelsCached = false
	normalizeModels(providerToPersist)
	providerData, err := json.MarshalIndent(providerToPersist, "", "  ")
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

	normalized := make([]catalog.Model, len(models))
	for index, model := range models {
		normalized[index] = cloneModel(model)
	}
	for index := range normalized {
		catalog.NormalizeReasoningCapabilities(&normalized[index])
		if normalized[index].MaxOutputTokens <= 0 {
			normalized[index].MaxOutputTokens = catalog.DefaultMaxOutputTokens
		}
	}
	if normalized == nil {
		normalized = make([]catalog.Model, 0)
	}

	r.modelCache[code] = modelDiscoveryCache{
		ExpiresAt: expiresAt.UTC(),
		Models:    normalized,
	}
	return nil
}

func (r *CatalogRepository) applyModelDiscoveryCache(provider *catalog.Provider) {
	cache, ok := r.modelCache[provider.Code]
	if !ok {
		return
	}
	configuredCodes := make(map[shared.ModelCode]struct{}, len(provider.Models))
	for _, model := range provider.Models {
		configuredCodes[model.Code] = struct{}{}
	}
	for _, model := range cache.Models {
		if _, exists := configuredCodes[model.Code]; exists {
			continue
		}
		provider.Models = append(provider.Models, cloneModel(model))
		provider.ModelsCached = true
	}
}

func (r *CatalogRepository) invalidateModelDiscoveryCacheLocked(code shared.Code) error {
	delete(r.modelCache, code)
	return nil
}

func providerConfigurationEqual(left, right *catalog.Provider) bool {
	return left.Code == right.Code &&
		left.Name == right.Name &&
		left.Type == right.Type &&
		left.BaseURL == right.BaseURL &&
		left.APIKey == right.APIKey &&
		left.Builtin == right.Builtin &&
		left.Official == right.Official &&
		left.FreeFormTool == right.FreeFormTool &&
		left.ModelsURL == right.ModelsURL &&
		left.TokenCountURL == right.TokenCountURL &&
		reflect.DeepEqual(left.Metadata, right.Metadata)
}

func mergeConfiguredAndEditedModels(
	configured []catalog.Model,
	incoming []catalog.Model,
	cached []catalog.Model,
) []catalog.Model {
	result := make([]catalog.Model, 0, len(configured)+len(incoming))
	indices := make(map[shared.ModelCode]int, len(configured)+len(incoming))
	incomingByCode := make(map[shared.ModelCode]catalog.Model, len(incoming))
	for _, model := range incoming {
		incomingByCode[model.Code] = model
	}
	for _, model := range configured {
		if _, exists := incomingByCode[model.Code]; !exists {
			continue
		}
		indices[model.Code] = len(result)
		result = append(result, cloneModel(model))
	}

	cachedByCode := make(map[shared.ModelCode]catalog.Model, len(cached))
	for _, model := range cached {
		cachedByCode[model.Code] = model
	}
	for _, model := range incoming {
		cachedModel, wasCached := cachedByCode[model.Code]
		if wasCached && modelsEqual(model, cachedModel) {
			continue
		}
		if index, exists := indices[model.Code]; exists {
			result[index] = cloneModel(model)
			continue
		}
		indices[model.Code] = len(result)
		result = append(result, cloneModel(model))
	}
	return result
}

func retainedCachedModels(cached, incoming []catalog.Model) []catalog.Model {
	incomingByCode := make(map[shared.ModelCode]catalog.Model, len(incoming))
	for _, model := range incoming {
		incomingByCode[model.Code] = model
	}
	retained := make([]catalog.Model, 0, len(cached))
	for _, model := range cached {
		incomingModel, exists := incomingByCode[model.Code]
		if exists && modelsEqual(model, incomingModel) {
			retained = append(retained, cloneModel(model))
		}
	}
	return retained
}

func modelsEqual(left, right catalog.Model) bool {
	return left.Code == right.Code &&
		left.Name == right.Name &&
		left.ContextWindow == right.ContextWindow &&
		left.MaxOutputTokens == right.MaxOutputTokens &&
		left.MultiModal == right.MultiModal &&
		left.Light == right.Light &&
		left.Reasoning == right.Reasoning &&
		slices.Equal(left.ReasoningEfforts, right.ReasoningEfforts) &&
		left.IsDefault == right.IsDefault &&
		left.CreatedAt.Equal(right.CreatedAt) &&
		left.UpdatedAt.Equal(right.UpdatedAt)
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
		copy.Models[index] = cloneModel(provider.Models[index])
		catalog.NormalizeReasoningCapabilities(&copy.Models[index])
	}
	copy.Metadata = maps.Clone(provider.Metadata)
	return &copy
}

func cloneModel(model catalog.Model) catalog.Model {
	model.ReasoningEfforts = slices.Clone(model.ReasoningEfforts)
	return model
}
