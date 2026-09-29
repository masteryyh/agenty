package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/api/routes"
	"github.com/masteryyh/agenty-core/pkg/application"
	domainmcp "github.com/masteryyh/agenty-core/pkg/domain/mcp"
	"github.com/masteryyh/agenty-core/pkg/infra/codexmode"
	"github.com/masteryyh/agenty-core/pkg/infra/compaction"
	"github.com/masteryyh/agenty-core/pkg/infra/config"
	"github.com/masteryyh/agenty-core/pkg/infra/httpapi"
	"github.com/masteryyh/agenty-core/pkg/infra/initialize"
	"github.com/masteryyh/agenty-core/pkg/infra/instance"
	"github.com/masteryyh/agenty-core/pkg/infra/logging"
	"github.com/masteryyh/agenty-core/pkg/infra/mcp"
	"github.com/masteryyh/agenty-core/pkg/infra/metadata"
	inframiddleware "github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/permission"
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

func reportCoreError(ctx context.Context, message string, attributes ...any) {
	slog.ErrorContext(ctx, message, attributes...)

	fmt.Fprintf(os.Stderr, "agenty-core: %s", message)
	for index := 0; index+1 < len(attributes); index += 2 {
		fmt.Fprintf(os.Stderr, " %v=%v", attributes[index], attributes[index+1])
	}
	if len(attributes)%2 != 0 {
		fmt.Fprintf(os.Stderr, " %v", attributes[len(attributes)-1])
	}
	fmt.Fprintln(os.Stderr)
}

func run() (exitCode int) {
	paths, err := config.ResolvePaths()
	if err != nil {
		fmt.Fprintln(os.Stderr, "agenty-core: failed to resolve data directory:", err)
		return 1
	}

	dataDir, err := instance.CanonicalDataDir(paths.DataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agenty-core: failed to canonicalize data directory:", err)
		return 1
	}
	if err := os.Setenv(config.EnvDataDir, dataDir); err != nil {
		fmt.Fprintln(os.Stderr, "agenty-core: failed to set data directory environment:", err)
		return 1
	}

	transport, address := instance.AddressForDataDir(dataDir)
	if configured := os.Getenv("AGENTY_TRANSPORT"); configured == "" || configured != transport {
		fmt.Fprintf(os.Stderr, "agenty-core: AGENTY_TRANSPORT must be %q for this platform\n", transport)
		return 1
	}
	if configured := os.Getenv("AGENTY_CORE_ADDR"); configured == "" || configured != address {
		fmt.Fprintln(os.Stderr, "agenty-core: AGENTY_CORE_ADDR does not match the data directory")
		return 1
	}
	if version := os.Getenv("AGENTY_IPC_VERSION"); version != "1" {
		fmt.Fprintln(os.Stderr, "agenty-core: unsupported or missing AGENTY_IPC_VERSION")
		return 1
	}

	coreLock, err := instance.Acquire(dataDir)
	if err != nil {
		if errors.Is(err, instance.ErrAlreadyRunning) {
			fmt.Fprintln(os.Stderr, "agenty-core: another core already owns this data directory")
			return 3
		}
		fmt.Fprintln(os.Stderr, "agenty-core: failed to lock data directory:", err)
		return 1
	}
	defer func() {
		if err := coreLock.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "agenty-core: failed to release data directory lock:", err)
			exitCode = 1
		}
	}()

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
	instance.MonitorParent(ctx, cancel)

	repos, err := initialize.OpenRepositories(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to open repositories", "error", err)
		return 1
	}
	defer func() {
		if err := repos.Close(); err != nil {
			reportCoreError(ctx, "failed to close repositories", "error", err)
			exitCode = 1
		}
	}()

	slog.InfoContext(ctx, "agenty-core started", "dataDir", dataDir)
	toolRegistry := infratools.NewRegistry()
	if err := builtin.RegisterAll(toolRegistry); err != nil {
		reportCoreError(ctx, "failed to register built-in tools", "error", err)
		return 1
	}

	var sessionService *application.SessionService
	var execution *infrasession.Engine
	var mcpRegistry *mcp.Registry
	var permissionManager *permission.PermissionManager
	eventBroker := httpapi.NewStreamBroker(func(snapshotCtx context.Context, topic string) (any, error) {
		if topic == "mcp" {
			return map[string]any{"servers": mcpRegistry.List(snapshotCtx)}, nil
		}
		sessionID := topic[len("session:"):]
		session, err := sessionService.Get(snapshotCtx, sessionID)
		if err != nil {
			return nil, err
		}
		id, err := uuid.Parse(sessionID)
		if err != nil {
			return nil, err
		}
		roundID, running := execution.ActiveRoundID(id)
		return map[string]any{
			"session":               session,
			"isRunning":             running,
			"roundId":               roundID,
			"pendingApprovals":      permissionManager.PendingForSession(id),
			"pendingPermissionMode": execution.PendingPermissionMode(id),
		}, nil
	})
	mcpRegistry, err = mcp.NewRegistry(ctx, config.Get().Paths().MCPDir, toolRegistry, mcp.Options{
		Events: func(eventCtx context.Context, event domainmcp.Event) {
			if err := eventBroker.Publish(eventCtx, "mcp", event); err != nil {
				slog.DebugContext(eventCtx, "failed to publish MCP event", "error", err)
			}
		},
	})
	if err != nil {
		reportCoreError(ctx, "failed to initialize MCP registry", "error", err)
		return 1
	}
	mcpRegistry.Start()
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := mcpRegistry.Shutdown(shutdownCtx); err != nil {
			reportCoreError(shutdownCtx, "failed to stop MCP registry", "error", err)
			exitCode = 1
		}
	}()

	permissionManager = permission.NewPermissionManager()
	middlewareManager := inframiddleware.NewManager()
	if err := middlewareManager.Register(skill.NewMiddleware(skillRegistry)); err != nil {
		reportCoreError(ctx, "failed to register skill middleware", "error", err)
		return 1
	}
	if err := middlewareManager.Register(mcp.NewMiddleware(mcpRegistry)); err != nil {
		reportCoreError(ctx, "failed to register MCP middleware", "error", err)
		return 1
	}
	if err := middlewareManager.Register(codexmode.NewMiddleware(codexmode.Config{})); err != nil {
		reportCoreError(ctx, "failed to register Codex Mode middleware", "error", err)
		return 1
	}
	if err := middlewareManager.Register(metadata.NewMiddleware()); err != nil {
		reportCoreError(ctx, "failed to register metadata middleware", "error", err)
		return 1
	}
	if err := middlewareManager.Register(compaction.NewMiddleware()); err != nil {
		reportCoreError(ctx, "failed to register compaction middleware", "error", err)
		return 1
	}
	if err := middlewareManager.Register(infratools.NewValidationMiddleware()); err != nil {
		reportCoreError(ctx, "failed to register tool input validation middleware", "error", err)
		return 1
	}
	if err := middlewareManager.Register(storage.NewSessionMiddleware(repos.Conversation)); err != nil {
		reportCoreError(ctx, "failed to register session storage middleware", "error", err)
		return 1
	}
	if err := middlewareManager.Register(httpapi.NewSessionEventMiddleware(eventBroker)); err != nil {
		reportCoreError(ctx, "failed to register session event middleware", "error", err)
		return 1
	}
	if err := middlewareManager.Register(permissionManager.Middleware()); err != nil {
		reportCoreError(ctx, "failed to register HITL middleware", "error", err)
		return 1
	}
	middlewareChain, err := middlewareManager.Compile()
	if err != nil {
		reportCoreError(ctx, "failed to compile middleware", "error", err)
		return 1
	}
	middlewareChain.SetEventBarrier(eventBroker.TopicBarrier)
	execution, err = infrasession.NewEngine(ctx, infrasession.Dependencies{
		Sessions:              repos.Conversation,
		Catalog:               repos.Catalog,
		Tools:                 toolRegistry,
		LoopHooks:             middlewareChain.AgentLoopHooks(),
		Lifecycle:             middlewareChain.LifecycleHooks(),
		PermissionModeChanged: permissionManager.PermissionModeChanged,
	})
	if err != nil {
		reportCoreError(ctx, "failed to initialize execution engine", "error", err)
		return 1
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := execution.Shutdown(shutdownCtx); err != nil {
			reportCoreError(shutdownCtx, "failed to stop execution engine", "error", err)
			exitCode = 1
		}
	}()

	sessionService = application.NewSessionService(
		repos.Conversation,
		application.WithSessionExecutionState(execution),
	)
	providerService := application.NewProviderService(repos.Catalog)
	initializeService := application.NewInitializeService(providerService, config.Get())
	apiDependencies := routes.Dependencies{
		Provider:       providerService,
		Initialization: initializeService,
		Session:        sessionService,
		Execution:      execution,
		MCP:            mcpRegistry,
		Skill:          skillRegistry,
		Permission:     permissionManager,
	}
	shutdownRequests := make(chan struct{}, 1)
	api := routes.NewServer(apiDependencies, routes.SystemInfo{
		DataDir:     dataDir,
		IPCVersion:  "1",
		APIContract: "v1",
		ProcessID:   os.Getpid(),
	}, eventBroker, func() {
		select {
		case shutdownRequests <- struct{}{}:
		default:
		}
	})

	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{
		Handler:           api,
		Protocols:         protocols,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	listener, err := instance.Listen(transport, address)
	if err != nil {
		reportCoreError(ctx, "failed to start local HTTP/2 listener", "transport", transport, "address", address, "error", err)
		return 1
	}
	defer listener.Close()
	metadataPath, err := instance.WriteMetadata(dataDir, instance.RuntimeMetadata{
		DataDir:       dataDir,
		Transport:     transport,
		Address:       address,
		IPCVersion:    "1",
		Protocol:      "h2c",
		ProcessID:     os.Getpid(),
		EventStreamID: eventBroker.StreamID(),
	})
	if err != nil {
		reportCoreError(ctx, "failed to write runtime metadata", "error", err)
		return 1
	}
	defer os.Remove(metadataPath)

	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.Serve(listener)
	}()
	slog.InfoContext(ctx, "agenty core ready", "dataDir", dataDir, "transport", transport, "address", address, "ipcVersion", "1")
	select {
	case err := <-serveErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			reportCoreError(ctx, "HTTP/2 server stopped with an error", "error", err)
			exitCode = 1
		}
	case <-ctx.Done():
	case <-shutdownRequests:
	}
	api.BeginShutdown()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := execution.Shutdown(shutdownCtx); err != nil {
		reportCoreError(shutdownCtx, "failed to stop execution engine", "error", err)
		exitCode = 1
	}
	if err := mcpRegistry.Shutdown(shutdownCtx); err != nil {
		reportCoreError(shutdownCtx, "failed to stop MCP registry", "error", err)
		exitCode = 1
	}
	eventBroker.Close()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.WarnContext(shutdownCtx, "HTTP/2 graceful shutdown timed out; closing remaining connections", "error", err)
		if closeErr := server.Close(); closeErr != nil {
			reportCoreError(shutdownCtx, "failed to close HTTP/2 server", "error", closeErr)
			exitCode = 1
		}
	}
	return exitCode
}
