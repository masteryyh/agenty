package catalog

import (
	"context"
	"errors"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

var ErrProviderNotFound = errors.New("catalog: provider not found")

type Repository interface {
	Get(ctx context.Context, code shared.Code) (*Provider, error)
	List(ctx context.Context) ([]*Provider, error)
	Save(ctx context.Context, provider *Provider) error
	Delete(ctx context.Context, code shared.Code) error
}
