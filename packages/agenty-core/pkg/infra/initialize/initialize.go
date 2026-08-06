package initialize

import (
	"context"
	"database/sql"

	"github.com/masteryyh/agenty-core/pkg/infra/config"
	"github.com/masteryyh/agenty-core/pkg/infra/storage"
)

func DataDir() (*config.Paths, error) {
	mgr, err := config.Init()
	if err != nil {
		return nil, err
	}
	return mgr.Paths(), nil
}

type Repositories struct {
	Conversation *storage.ConversationRepository
	Agent        *storage.AgentRepository
	Catalog      *storage.CatalogRepository
	db           *sql.DB
}

func (r *Repositories) Close() error {
	if r.db == nil {
		return nil
	}
	return r.db.Close()
}

func OpenRepositories(ctx context.Context) (*Repositories, error) {
	paths, err := DataDir()
	if err != nil {
		return nil, err
	}

	db, err := storage.OpenDB(paths.DatabaseFile)
	if err != nil {
		return nil, err
	}

	return &Repositories{
		Conversation: storage.NewConversationRepository(db, paths.SessionsDir),
		Agent:        storage.NewAgentRepository(paths.AgentsDir),
		Catalog:      storage.NewCatalogRepository(paths.ProvidersDir),
		db:           db,
	}, nil
}
