package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

var ErrProviderNotFound = errors.New("storage: provider not found")

type CatalogRepository struct {
	providersDir string
}

func NewCatalogRepository(providersDir string) *CatalogRepository {
	return &CatalogRepository{providersDir: providersDir}
}

func (r *CatalogRepository) Get(ctx context.Context, slug shared.Slug) (*catalog.Provider, error) {
	providerPath := filepath.Join(r.providersDir, slug.String(), "provider.json")
	data, err := os.ReadFile(providerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrProviderNotFound
		}
		return nil, err
	}

	var p catalog.Provider
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}

	modelsDir := filepath.Join(r.providersDir, slug.String(), "models")
	entries, err := os.ReadDir(modelsDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		modelData, err := os.ReadFile(filepath.Join(modelsDir, entry.Name()))
		if err != nil {
			return nil, err
		}

		var m catalog.Model
		if err := json.Unmarshal(modelData, &m); err != nil {
			return nil, err
		}
		p.Models = append(p.Models, m)
	}

	return &p, nil
}

func (r *CatalogRepository) List(ctx context.Context) ([]*catalog.Provider, error) {
	entries, err := os.ReadDir(r.providersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var providers []*catalog.Provider
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		slug, err := shared.NewSlug(entry.Name())
		if err != nil {
			continue
		}

		p, err := r.Get(ctx, slug)
		if err != nil {
			if errors.Is(err, ErrProviderNotFound) {
				continue
			}
			return nil, err
		}
		providers = append(providers, p)
	}
	return providers, nil
}

func (r *CatalogRepository) Save(ctx context.Context, provider *catalog.Provider) error {
	providerDir := filepath.Join(r.providersDir, provider.Slug.String())
	if err := os.MkdirAll(providerDir, 0700); err != nil {
		return err
	}

	type providerFile struct {
		Slug      shared.Slug     `json:"slug"`
		Name      string          `json:"name"`
		Type      catalog.APIType `json:"type"`
		BaseURL   string          `json:"baseUrl"`
		APIKey    string          `json:"apiKey"`
		Metadata  shared.Metadata `json:"metadata,omitempty"`
		CreatedAt time.Time       `json:"createdAt"`
		UpdatedAt time.Time       `json:"updatedAt"`
	}
	pf := providerFile{
		Slug:      provider.Slug,
		Name:      provider.Name,
		Type:      provider.Type,
		BaseURL:   provider.BaseURL,
		APIKey:    provider.APIKey,
		Metadata:  provider.Metadata,
		CreatedAt: provider.CreatedAt,
		UpdatedAt: provider.UpdatedAt,
	}
	providerData, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return err
	}

	providerPath := filepath.Join(providerDir, "provider.json")
	if err := os.WriteFile(providerPath, providerData, 0600); err != nil {
		return err
	}

	modelsDir := filepath.Join(providerDir, "models")
	if err := os.MkdirAll(modelsDir, 0700); err != nil {
		return err
	}

	for _, model := range provider.Models {
		modelData, err := json.MarshalIndent(model, "", "    ")
		if err != nil {
			return err
		}

		modelPath := filepath.Join(modelsDir, model.Slug.String()+".json")
		if err := os.WriteFile(modelPath, modelData, 0600); err != nil {
			return err
		}
	}

	return nil
}

func (r *CatalogRepository) Delete(ctx context.Context, slug shared.Slug) error {
	providerDir := filepath.Join(r.providersDir, slug.String())
	if _, err := os.Stat(providerDir); err != nil {
		if os.IsNotExist(err) {
			return ErrProviderNotFound
		}
		return err
	}
	if err := os.RemoveAll(providerDir); err != nil {
		return err
	}
	return nil
}

func (r *CatalogRepository) DeleteModel(ctx context.Context, providerSlug, modelSlug shared.Slug) error {
	modelPath := filepath.Join(r.providersDir, providerSlug.String(), "models", modelSlug.String()+".json")
	if err := os.Remove(modelPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	providerDir := filepath.Join(r.providersDir, providerSlug.String())
	if _, derr := os.Stat(providerDir); os.IsNotExist(derr) {
		return ErrProviderNotFound
	}
	return catalog.ErrModelNotFound
}
