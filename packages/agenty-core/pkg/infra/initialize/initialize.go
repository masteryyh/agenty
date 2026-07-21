package initialize

import (
	"context"

	"github.com/masteryyh/agenty-core/pkg/infra/config"
	"github.com/masteryyh/agenty-core/pkg/infra/storage"
)

func DataDir() (*config.Paths, error) {
	if err := config.InitializeDataDir(); err != nil {
		return nil, err
	}
	return config.ResolvePaths()
}

type Repositories struct {
	Conversation *storage.ConversationRepository
	Agent        *storage.AgentRepository
	Catalog      *storage.CatalogRepository
}

func (r *Repositories) Close() error {
	return storage.CloseDB()
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
	}, nil
}
