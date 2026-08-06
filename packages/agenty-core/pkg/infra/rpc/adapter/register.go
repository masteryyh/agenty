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
	sessionSvc *application.SessionService,
	execution *agentloop.Engine,
) {
	RegisterAgentHandlers(d, agentSvc)
	RegisterProviderHandlers(d, providerSvc)
	RegisterSessionHandlers(d, sessionSvc, execution)
}
