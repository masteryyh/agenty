package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/agentloop/builtin"
	"github.com/masteryyh/agenty-core/pkg/application"
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/infra/config"
	"github.com/masteryyh/agenty-core/pkg/infra/initialize"
	"github.com/masteryyh/agenty-core/pkg/infra/llm"
	"github.com/masteryyh/agenty-core/pkg/infra/logging"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc/adapter"
	"github.com/masteryyh/agenty-core/pkg/infra/skill"
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
	dataDir, err := filepath.Abs(config.Get().Paths().DataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agenty-core: failed to resolve data directory:", err)
		return 1
	}
	if err := os.Setenv(config.EnvDataDir, dataDir); err != nil {
		fmt.Fprintln(os.Stderr, "agenty-core: failed to set data directory environment:", err)
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

	skillRegistry, err := skill.Scan(config.Get().Paths().SkillsDir)
	if err != nil {
		slog.Warn("failed to scan skills", "error", err)
		skillRegistry = nil
	} else {
		for _, diagnostic := range skillRegistry.Diagnostics() {
			slog.Warn("skill discovery diagnostic", "code", diagnostic.Code, "message", diagnostic.Message, "path", diagnostic.Path)
		}
	}

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

	slog.InfoContext(ctx, "agenty-core started", "dataDir", dataDir)
	toolRegistry := agentloop.NewRegistry()
	if err := builtin.RegisterAll(toolRegistry); err != nil {
		slog.ErrorContext(ctx, "failed to register built-in tools", "error", err)
		return 1
	}

	disp := rpc.NewDispatcher()
	srv := rpc.NewServer(disp, os.Stdin, os.Stdout)
	execution, err := agentloop.NewEngine(ctx, agentloop.Dependencies{
		Sessions: repos.Conversation,
		Catalog:  repos.Catalog,
		Tools:    toolRegistry,
		NewCaller: func(
			callerCtx context.Context,
			provider catalog.Provider,
			model catalog.Model,
		) (agentloop.Caller, error) {
			return llm.NewCaller(callerCtx, provider, model)
		},
		Events: func(eventCtx context.Context, event agentloop.SessionEvent) error {
			return srv.Notify(eventCtx, "session.event", event)
		},
		Compactions: func(eventCtx context.Context, event agentloop.CompactionEvent) error {
			return srv.Notify(eventCtx, "session.compaction", event)
		},
		Skills: skillRegistry,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to initialize execution engine", "error", err)
		return 1
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := execution.Shutdown(shutdownCtx); err != nil {
			slog.ErrorContext(shutdownCtx, "failed to stop execution engine", "error", err)
			exitCode = 1
		}
	}()

	sessionService := application.NewSessionService(
		repos.Conversation,
		application.WithSessionExecutionState(execution),
	)
	providerService := application.NewProviderService(repos.Catalog)
	initializeService := application.NewInitializeService(providerService, config.Get())
	adapter.RegisterAll(disp,
		providerService,
		initializeService,
		sessionService,
		execution,
	)
	adapter.RegisterSkillHandlers(disp, skillRegistry)

	asm := rpc.NewChunkAssembler(disp)
	rpc.RegisterChunkHandlers(disp, asm)
	asm.StartCleanup(ctx)

	if err := srv.Serve(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.ErrorContext(ctx, "server stopped with an error", "error", err)
		return 1
	}
	return 0
}
