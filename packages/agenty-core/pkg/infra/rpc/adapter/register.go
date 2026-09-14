package adapter

import (
	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
	infrasession "github.com/masteryyh/agenty-core/pkg/infra/session"
)

type codeParams struct {
	Code string `json:"code"`
}

func RegisterAll(
	d *rpc.Dispatcher,
	providerSvc *application.ProviderService,
	initializeSvc *application.InitializeService,
	sessionSvc *application.SessionService,
	execution *infrasession.Engine,
) {
	RegisterProviderHandlers(d, providerSvc)
	RegisterInitializeHandlers(d, initializeSvc)
	RegisterSessionHandlers(d, sessionSvc, execution)
}
