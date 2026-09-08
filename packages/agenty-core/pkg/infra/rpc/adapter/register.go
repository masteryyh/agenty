package adapter

import (
	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
)

type codeParams struct {
	Code string `json:"code"`
}

func RegisterAll(
	d *rpc.Dispatcher,
	providerSvc *application.ProviderService,
	initializeSvc *application.InitializeService,
	sessionSvc *application.SessionService,
	execution *agentloop.Engine,
) {
	RegisterProviderHandlers(d, providerSvc)
	RegisterInitializeHandlers(d, initializeSvc)
	RegisterSessionHandlers(d, sessionSvc, execution)
}
