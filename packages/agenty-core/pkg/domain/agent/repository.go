package agent

import (
	"context"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type Repository interface {
	Get(ctx context.Context, slug shared.Slug) (*Agent, error)
	List(ctx context.Context) ([]*Agent, error)
	Save(ctx context.Context, agent *Agent) error
	Delete(ctx context.Context, slug shared.Slug) error
	Default(ctx context.Context) (*Agent, error)
}
