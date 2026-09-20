package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/masteryyh/agenty-core/pkg/application"
	domainmcp "github.com/masteryyh/agenty-core/pkg/domain/mcp"
	"github.com/masteryyh/agenty-core/pkg/infra/compaction"
	"github.com/masteryyh/agenty-core/pkg/infra/config"
	"github.com/masteryyh/agenty-core/pkg/infra/initialize"
	"github.com/masteryyh/agenty-core/pkg/infra/logging"
	"github.com/masteryyh/agenty-core/pkg/infra/mcp"
	"github.com/masteryyh/agenty-core/pkg/infra/metadata"
	inframiddleware "github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/permission"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc"
	"github.com/masteryyh/agenty-core/pkg/infra/rpc/adapter"
	infrasession "github.com/masteryyh/agenty-core/pkg/infra/session"
	"github.com/masteryyh/agenty-core/pkg/infra/skill"
	"github.com/masteryyh/agenty-core/pkg/infra/storage"
	infratools "github.com/masteryyh/agenty-core/pkg/infra/tools"
	"github.com/masteryyh/agenty-core/pkg/infra/tools/builtin"
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
	toolRegistry := infratools.NewRegistry()
	if err := builtin.RegisterAll(toolRegistry); err != nil {
		slog.ErrorContext(ctx, "failed to register built-in tools", "error", err)
		return 1
	}

	disp := rpc.NewDispatcher()
	srv := rpc.NewServer(disp, os.Stdin, os.Stdout)
	mcpRegistry, err := mcp.NewRegistry(ctx, config.Get().Paths().MCPDir, toolRegistry, mcp.Options{
		Events: func(eventCtx context.Context, event domainmcp.Event) {
			if err := srv.Notify(eventCtx, "mcp.event", event); err != nil {
				slog.DebugContext(eventCtx, "failed to publish MCP event", "error", err)
			}
		},
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to initialize MCP registry", "error", err)
		return 1
	}
	mcpRegistry.Start()
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := mcpRegistry.Shutdown(shutdownCtx); err != nil {
			slog.ErrorContext(shutdownCtx, "failed to stop MCP registry", "error", err)
			exitCode = 1
		}
	}()

	permission := permission.NewPermissionManager()
	middlewareManager := inframiddleware.NewManager()
	if err := middlewareManager.Register(skill.NewMiddleware(skillRegistry)); err != nil {
		slog.ErrorContext(ctx, "failed to register skill middleware", "error", err)
		return 1
	}
	if err := middlewareManager.Register(mcp.NewMiddleware(mcpRegistry)); err != nil {
		slog.ErrorContext(ctx, "failed to register MCP middleware", "error", err)
		return 1
	}
	if err := middlewareManager.Register(metadata.NewMiddleware()); err != nil {
		slog.ErrorContext(ctx, "failed to register metadata middleware", "error", err)
		return 1
	}
	if err := middlewareManager.Register(compaction.NewMiddleware()); err != nil {
		slog.ErrorContext(ctx, "failed to register compaction middleware", "error", err)
		return 1
	}
	if err := middlewareManager.Register(storage.NewSessionMiddleware(repos.Conversation)); err != nil {
		slog.ErrorContext(ctx, "failed to register session storage middleware", "error", err)
		return 1
	}
	if err := middlewareManager.Register(rpc.NewSessionNotificationMiddleware(srv.Notify)); err != nil {
		slog.ErrorContext(ctx, "failed to register session notification middleware", "error", err)
		return 1
	}
	if err := middlewareManager.Register(permission.Middleware()); err != nil {
		slog.ErrorContext(ctx, "failed to register HITL middleware", "error", err)
		return 1
	}
	middlewareChain, err := middlewareManager.Compile()
	if err != nil {
		slog.ErrorContext(ctx, "failed to compile middleware", "error", err)
		return 1
	}
	execution, err := infrasession.NewEngine(ctx, infrasession.Dependencies{
		Sessions:              repos.Conversation,
		Catalog:               repos.Catalog,
		Tools:                 toolRegistry,
		LoopHooks:             middlewareChain.AgentLoopHooks(),
		Lifecycle:             middlewareChain.LifecycleHooks(),
		PermissionModeChanged: permission.PermissionModeChanged,
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
	adapter.RegisterMCPHandlers(disp, mcpRegistry)
	adapter.RegisterSkillHandlers(disp, skillRegistry)
	adapter.RegisterHITLHandlers(disp, permission)

	asm := rpc.NewChunkAssembler(disp)
	rpc.RegisterChunkHandlers(disp, asm)
	asm.StartCleanup(ctx)

	if err := srv.Serve(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.ErrorContext(ctx, "server stopped with an error", "error", err)
		return 1
	}
	return 0
}
