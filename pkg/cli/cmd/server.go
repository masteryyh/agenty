/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty/pkg/chat/sessionhooks"
	"github.com/masteryyh/agenty/pkg/config"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/gateway"
	mcppkg "github.com/masteryyh/agenty/pkg/mcp"
	"github.com/masteryyh/agenty/pkg/middleware"
	"github.com/masteryyh/agenty/pkg/routes"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/tools"
	"github.com/masteryyh/agenty/pkg/tools/builtin"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"github.com/masteryyh/agenty/pkg/utils/signal"
)

func startServer() error {
	cfg := config.GetConfigManager().GetConfig()

	slog.Info("starting agenty server...")

	baseCtx, cancel := signal.SetupContext()
	defer cancel()

	slog.InfoContext(baseCtx, "initializing database connection...")
	if err := conn.InitDB(baseCtx, cfg.DB, cfg.Debug); err != nil {
		return fmt.Errorf("failed to initialize database connection: %w", err)
	}

	slog.InfoContext(baseCtx, "registering built-in tools...")
	registry := tools.GetRegistry()
	builtin.RegisterAll(registry)
	sessionhooks.RegisterAll()
	slog.InfoContext(baseCtx, "built-in tools registered", "count", len(registry.All()))

	slog.InfoContext(baseCtx, "initializing MCP manager...")
	mcpManager := mcppkg.InitManager(baseCtx, registry)
	mcpManager.Start()
	slog.InfoContext(baseCtx, "MCP manager initialized")

	slog.InfoContext(baseCtx, "initializing skill service...")
	skillSvc := services.GetSkillService()
	var gatewayManager *gateway.Manager
	var httpServer *http.Server
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if httpServer != nil {
			if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.WarnContext(shutdownCtx, "failed to shutdown http server", "error", err)
			}
		}
		if gatewayManager != nil {
			gatewayManager.Stop(shutdownCtx)
		}
		skillSvc.Shutdown()
		mcpManager.Close()
	}()

	if err := skillSvc.Initialize(baseCtx); err != nil {
		slog.WarnContext(baseCtx, "skill service initialization failed", "error", err)
	}

	slog.InfoContext(baseCtx, "initializing gateway manager...")
	gatewayManager = gateway.NewManager()
	if err := gatewayManager.Start(baseCtx); err != nil {
		return fmt.Errorf("failed to start gateway manager: %w", err)
	}

	if cfg.Debug {
		gin.SetMode(gin.DebugMode)
		gin.DebugPrintRouteFunc = func(httpMethod string, absolutePath string, handlerName string, handlers int) {
			slog.DebugContext(baseCtx, "registered http route", "method", httpMethod, "path", absolutePath, "handler", handlerName, "handlers", handlers)
		}
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	if cfg.Debug {
		engine.Use(middleware.RequestLoggerMiddleware())
	}
	engine.Use(middleware.RecoveryMiddleware())
	engine.Use(middleware.CORSMiddleware())

	if cfg.Auth != nil && cfg.Auth.Enabled {
		slog.InfoContext(baseCtx, "HTTP Basic Auth enabled")
		engine.Use(middleware.BasicAuthMiddleware(cfg.Auth))
	}

	apiRoute := engine.Group("/api")
	v1Route := routes.GetV1Routes()
	if err := v1Route.RegisterRoutes(apiRoute.Group("/v1")); err != nil {
		return fmt.Errorf("failed to register routes: %w", err)
	}

	port := cfg.Port
	httpServer = &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: engine,
	}
	serverErr := make(chan error, 1)
	safe.GoOnce("http-server", func() {
		slog.InfoContext(baseCtx, "starting http server", "port", port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	})

	select {
	case <-baseCtx.Done():
		slog.InfoContext(baseCtx, "shutting down server")
		return nil
	case err := <-serverErr:
		return fmt.Errorf("failed to start http server: %w", err)
	}
}
