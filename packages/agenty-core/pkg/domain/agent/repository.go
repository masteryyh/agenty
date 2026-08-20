package agent

import (
	"context"
	"errors"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

var ErrNotFound = errors.New("agent: not found")

type Repository interface {
	Get(ctx context.Context, code shared.Code) (*Agent, error)
	List(ctx context.Context) ([]*Agent, error)
	Save(ctx context.Context, agent *Agent) error
	Delete(ctx context.Context, code shared.Code) error
	Default(ctx context.Context) (*Agent, error)
}
