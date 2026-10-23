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
	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	domainlocal "github.com/masteryyh/agenty-core/pkg/domain/localinference"
	domainmcp "github.com/masteryyh/agenty-core/pkg/domain/mcp"
	"github.com/masteryyh/agenty-core/pkg/infra/codexmode"
	"github.com/masteryyh/agenty-core/pkg/infra/compaction"
	"github.com/masteryyh/agenty-core/pkg/infra/config"
	"github.com/masteryyh/agenty-core/pkg/infra/event"
	"github.com/masteryyh/agenty-core/pkg/infra/initialize"
	"github.com/masteryyh/agenty-core/pkg/infra/instance"
	localruntime "github.com/masteryyh/agenty-core/pkg/infra/localinference"
	"github.com/masteryyh/agenty-core/pkg/infra/logging"
	"github.com/masteryyh/agenty-core/pkg/infra/mcp"
	"github.com/masteryyh/agenty-core/pkg/infra/metadata"
	inframiddleware "github.com/masteryyh/agenty-core/pkg/infra/middleware"
	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
	"github.com/masteryyh/agenty-core/pkg/infra/modelhub"
	"github.com/masteryyh/agenty-core/pkg/infra/permission"
	infrasession "github.com/masteryyh/agenty-core/pkg/infra/session"
	"github.com/masteryyh/agenty-core/pkg/infra/skill"
	"github.com/masteryyh/agenty-core/pkg/infra/storage"
	infratools "github.com/masteryyh/agenty-core/pkg/infra/tools"
	"github.com/masteryyh/agenty-core/pkg/infra/tools/builtin"
	"github.com/masteryyh/agenty-core/pkg/utils/signal"
	"path/filepath"
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
	var localManager *localruntime.Manager
	var workerInstaller *localruntime.Installer
	var modelDownloads *modelhub.Manager
	eventBroker := event.NewStreamBroker(func(snapshotCtx context.Context, topic string) (any, error) {
		if topic == "localInference" {
			return map[string]any{"runtime": localManager.Snapshot(), "installations": workerInstaller.List(), "modelDownloads": modelDownloads.List()}, nil
		}
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
			"session":          session,
			"isRunning":        running,
			"roundId":          roundID,
			"pendingApprovals": permissionManager.PendingForSession(id),
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

	localManager = localruntime.New(ctx, func(snapshot domainlocal.Snapshot) {
		_ = eventBroker.Publish(ctx, "localInference", map[string]any{"runtime": snapshot})
	})
	workerInstaller = localruntime.NewInstaller(ctx, filepath.Join(dataDir, "workers"), func(installation localruntime.Installation) {
		_ = eventBroker.Publish(ctx, "localInference", map[string]any{"installation": installation})
	}, func(worker domainlocal.Worker) error {
		_, err := localManager.UpdateConfig(config.Get(), func(cfg *domainlocal.Config) {
			for index := range cfg.Workers {
				if cfg.Workers[index].Backend == worker.Backend {
					cfg.Workers[index] = worker
					return
				}
			}
			cfg.Workers = append(cfg.Workers, worker)
		})
		return err
	})
	if err := localManager.Configure(config.Get().LocalInference()); err != nil {
		slog.WarnContext(ctx, "local inference configuration is invalid", "error", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		_ = workerInstaller.Shutdown(shutdownCtx)
		_ = localManager.Shutdown(shutdownCtx)
	}()
	modelClient := &modelcall.Client{LocalRuntime: localManager}
	permissionManager = permission.NewPermissionManager(modelClient)
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
	if err := middlewareManager.Register(event.NewSessionEventMiddleware(eventBroker)); err != nil {
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
		InvokeModel: modelClient.Invoke,
		Sessions:    repos.Conversation,
		Catalog:     repos.Catalog,
		Tools:       toolRegistry,
		LoopHooks:   middlewareChain.AgentLoopHooks(),
		Lifecycle:   middlewareChain.LifecycleHooks(),
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
		application.WithSessionCatalog(repos.Catalog),
	)
	providerService := application.NewProviderService(repos.Catalog)
	providerService.SetLocalEnabled(func() bool { return config.Get().LocalInference().Enabled })
	modelDownloads = modelhub.NewManager(ctx, modelhub.Options{
		Directory: filepath.Join(dataDir, "models"),
		Workers:   func() []domainlocal.Worker { return config.Get().LocalInference().Workers },
		Publish: func(download modelhub.Download) {
			_ = eventBroker.Publish(ctx, "localInference", map[string]any{"modelDownload": download})
		},
		Register: func(registerCtx context.Context, code, name string, contextWindow int, model *domainlocal.Model) error {
			probeCtx, probeCancel := context.WithTimeout(registerCtx, 2*time.Minute)
			defer probeCancel()
			if _, err := localManager.Probe(probeCtx, model); err != nil {
				return err
			}
			_, err := providerService.AddModel(registerCtx, "local", code, application.ModelInput{Name: name, ContextWindow: contextWindow, MaxOutputTokens: 512, Local: model})
			return err
		},
	})
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		_ = modelDownloads.Shutdown(shutdownCtx)
	}()
	initializeService := application.NewInitializeService(providerService, config.Get())
	initializeService.SetLocalReadiness(func(ctx context.Context, model catalog.Model) error {
		if !config.Get().LocalInference().Enabled {
			return fmt.Errorf("enable local inference before selecting a local default model")
		}
		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		_, err := localManager.Probe(probeCtx, model.Local)
		return err
	})
	apiDependencies := routes.Dependencies{
		LocalInference:  localManager,
		WorkerInstaller: workerInstaller,
		ModelDownloads:  modelDownloads,
		Config:          config.Get(),
		Provider:        providerService,
		Initialization:  initializeService,
		Session:         sessionService,
		Execution:       execution,
		MCP:             mcpRegistry,
		Skill:           skillRegistry,
		Permission:      permissionManager,
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
	if err := workerInstaller.Shutdown(shutdownCtx); err != nil {
		slog.WarnContext(shutdownCtx, "failed to stop worker installer", "error", err)
	}
	if err := localManager.Shutdown(shutdownCtx); err != nil {
		slog.WarnContext(shutdownCtx, "failed to stop local inference", "error", err)
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
