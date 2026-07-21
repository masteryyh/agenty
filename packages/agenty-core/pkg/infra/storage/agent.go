package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/agent"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

var ErrAgentNotFound = errors.New("storage: agent not found")

type AgentRepository struct {
	agentsDir string
}

func NewAgentRepository(agentsDir string) *AgentRepository {
	return &AgentRepository{agentsDir: agentsDir}
}

func (r *AgentRepository) Get(ctx context.Context, slug shared.Slug) (*agent.Agent, error) {
	path := filepath.Join(r.agentsDir, slug.String()+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	var a agent.Agent
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *AgentRepository) List(ctx context.Context) ([]*agent.Agent, error) {
	entries, err := os.ReadDir(r.agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var agents []*agent.Agent
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		slugStr := entry.Name()[:len(entry.Name())-5]
		slug, err := shared.NewSlug(slugStr)
		if err != nil {
			continue
		}

		a, err := r.Get(ctx, slug)
		if errors.Is(err, ErrAgentNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, nil
}

func (r *AgentRepository) Save(ctx context.Context, a *agent.Agent) error {
	if err := os.MkdirAll(r.agentsDir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(a, "", "    ")
	if err != nil {
		return err
	}

	path := filepath.Join(r.agentsDir, a.Slug.String()+".json")
	return os.WriteFile(path, data, 0600)
}

func (r *AgentRepository) Delete(ctx context.Context, slug shared.Slug) error {
	path := filepath.Join(r.agentsDir, slug.String()+".json")
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return ErrAgentNotFound
	}
	return err
}

func (r *AgentRepository) Default(ctx context.Context) (*agent.Agent, error) {
	agents, err := r.List(ctx)
	if err != nil {
		return nil, err
	}

	for _, a := range agents {
		if a.IsDefault {
			return a, nil
		}
	}
	return nil, ErrAgentNotFound
}
