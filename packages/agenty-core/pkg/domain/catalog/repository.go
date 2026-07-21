package catalog

import (
	"context"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type Repository interface {
	Get(ctx context.Context, slug shared.Slug) (*Provider, error)
	List(ctx context.Context) ([]*Provider, error)
	Save(ctx context.Context, provider *Provider) error
	Delete(ctx context.Context, slug shared.Slug) error
}
