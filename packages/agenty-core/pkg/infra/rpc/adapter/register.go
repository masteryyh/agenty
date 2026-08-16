package adapter

import (
	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
)

func RegisterAll(
	d *rpc.Dispatcher,
	agentSvc *application.AgentService,
	providerSvc *application.ProviderService,
	initializeSvc *application.InitializeService,
	sessionSvc *application.SessionService,
	execution *agentloop.Engine,
) {
	RegisterAgentHandlers(d, agentSvc)
	RegisterProviderHandlers(d, providerSvc)
	RegisterInitializeHandlers(d, initializeSvc)
	RegisterSessionHandlers(d, sessionSvc, execution)
}
