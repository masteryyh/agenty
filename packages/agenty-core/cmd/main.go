package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/infra/initialize"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc/adapter"
	"github.com/masteryyh/agenty-core/pkg/utils/signal"
)

func main() {
	ctx, cancel := signal.SetupContext()
	defer cancel()

	repos, err := initialize.OpenRepositories(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agenty-core: failed to open repositories:", err)
		os.Exit(1)
	}
	defer repos.Close()

	disp := rpc.NewDispatcher()
	adapter.RegisterAll(disp,
		application.NewAgentService(repos.Agent),
		application.NewProviderService(repos.Catalog),
		application.NewSessionService(repos.Conversation),
	)

	asm := rpc.NewChunkAssembler(disp)
	rpc.RegisterChunkHandlers(disp, asm)
	asm.StartCleanup(ctx)

	srv := rpc.NewServer(disp, os.Stdin, os.Stdout)
	if err := srv.Serve(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "agenty-core: server error:", err)
		os.Exit(1)
	}
}
