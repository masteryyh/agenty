package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

var ErrProviderNotFound = catalog.ErrProviderNotFound

type CatalogRepository struct {
	providersDir string
}

func NewCatalogRepository(providersDir string) *CatalogRepository {
	return &CatalogRepository{providersDir: providersDir}
}

func (r *CatalogRepository) Get(_ context.Context, code shared.Code) (*catalog.Provider, error) {
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
	normalizeModels(&provider)

	return &provider, nil
}

func (r *CatalogRepository) List(ctx context.Context) ([]*catalog.Provider, error) {
	providers := make([]*catalog.Provider, 0)
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
	if err := os.MkdirAll(r.providersDir, 0700); err != nil {
		return err
	}

	normalizeModels(provider)
	providerData, err := json.MarshalIndent(provider, "", "  ")
	if err != nil {
		return err
	}

	providerPath := filepath.Join(r.providersDir, provider.Code.String()+".json")
	return os.WriteFile(providerPath, providerData, 0600)
}

func (r *CatalogRepository) Delete(_ context.Context, code shared.Code) error {
	providerPath := filepath.Join(r.providersDir, code.String()+".json")
	if err := os.Remove(providerPath); err != nil {
		if os.IsNotExist(err) {
			return ErrProviderNotFound
		}
		return err
	}
	return nil
}

func normalizeModels(provider *catalog.Provider) {
	if provider.Models == nil {
		provider.Models = make([]catalog.Model, 0)
	}
	for index := range provider.Models {
		provider.Models[index].MaxOutputTokens = catalog.DefaultMaxOutputTokens
	}
}
