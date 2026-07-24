package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/infra/config"
	"github.com/masteryyh/agenty-core/pkg/infra/initialize"
	"github.com/masteryyh/agenty-core/pkg/infra/logging"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc/adapter"
	"github.com/masteryyh/agenty-core/pkg/utils/signal"
)

func main() {
	os.Exit(run())
}

func run() (exitCode int) {
	if _, err := config.Init(); err != nil {
		fmt.Fprintln(os.Stderr, "agenty-core: failed to initialize config:", err)
		return 1
	}

	logger, err := logging.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, "agenty-core: failed to initialize logging:", err)
		return 1
	}
	slog.SetDefault(logger.Logger)
	defer func() {
		if err := logger.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "agenty-core: failed to close logging:", err)
			exitCode = 1
		}
	}()

	ctx, cancel := signal.SetupContext()
	defer cancel()

	repos, err := initialize.OpenRepositories(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to open repositories", "error", err)
		return 1
	}
	defer func() {
		if err := repos.Close(); err != nil {
			slog.ErrorContext(ctx, "failed to close repositories", "error", err)
			exitCode = 1
		}
	}()

	slog.InfoContext(ctx, "agenty-core started", "dataDir", config.Get().Paths().DataDir)

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
		slog.ErrorContext(ctx, "server stopped with an error", "error", err)
		return 1
	}
	return 0
}
