package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/infra/config"
	localruntime "github.com/masteryyh/agenty-core/pkg/infra/localinference"
	"github.com/masteryyh/agenty-core/pkg/infra/mcp"
	"github.com/masteryyh/agenty-core/pkg/infra/modelhub"
	"github.com/masteryyh/agenty-core/pkg/infra/permission"
	infrasession "github.com/masteryyh/agenty-core/pkg/infra/session"
	"github.com/masteryyh/agenty-core/pkg/infra/skill"
)

type Dependencies struct {
	LocalInference  *localruntime.Manager
	WorkerInstaller *localruntime.Installer
	ModelDownloads  *modelhub.Manager
	Config          *config.Manager
	Provider        *application.ProviderService
	Initialization  *application.InitializeService
	Session         *application.SessionService
	Execution       *infrasession.Engine
	MCP             *mcp.Registry
	Skill           *skill.Registry
	Permission      *permission.PermissionManager
}

type APIRoutes struct {
	dependencies   Dependencies
	initialization *InitializationRoutes
	provider       *ProviderRoutes
	session        *SessionRoutes
	skill          *SkillRoutes
	mcp            *MCPRoutes
	permission     *PermissionRoutes
}

func newAPIRoutes(api *API, deps Dependencies) *APIRoutes {
	return &APIRoutes{
		dependencies:   deps,
		initialization: newInitializationRoutes(api, deps.Initialization),
		provider:       newProviderRoutes(api, deps.Provider),
		session:        newSessionRoutes(api, deps.Session, deps.Execution),
		skill:          newSkillRoutes(api, deps.Skill),
		mcp:            newMCPRoutes(api, deps.MCP),
		permission:     newPermissionRoutes(api, deps.Permission),
	}
}

func (r *APIRoutes) RegisterRoutes(engine *gin.Engine, api *API) {
	v1 := engine.Group("/v1")
	registerSystemRoutes(v1, api)
	r.initialization.RegisterRoutes(v1)
	r.provider.RegisterRoutes(v1)
	r.session.RegisterRoutes(v1)
	r.skill.RegisterRoutes(v1)
	r.mcp.RegisterRoutes(v1)
	r.permission.RegisterRoutes(v1)
	registerStreamRoutes(v1, api)
	registerLocalInferenceRoutes(v1, api, r.dependencies)
	registerModelHubRoutes(v1, api, r.dependencies.ModelDownloads)
}
