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

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty/pkg/config"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/middleware"
	"github.com/masteryyh/agenty/pkg/routes"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"github.com/masteryyh/agenty/pkg/utils/signal"
)

func main() {
	slog.Info("starting agenty server...")

	slog.Info("loading configuration...")
	if err := config.Init(); err != nil {
		slog.Error("failed to load configuration", "error", err)
		return
	}
	cfg := config.GetConfigManager().GetConfig()

	baseCtx, cancel := signal.SetupContext()
	defer cancel()

	slog.InfoContext(baseCtx, "initializing database connection...")
	if err := conn.InitDB(baseCtx, cfg.DB); err != nil {
		slog.ErrorContext(baseCtx, "failed to initialize database connection", "error", err)
		return
	}

	engine := gin.New()
	engine.Use(middleware.RecoveryMiddleware())
	engine.Use(middleware.CORSMiddleware())

	apiRoute := engine.Group("/api")
	v1Route := routes.GetV1Routes()
	v1Route.RegisterRoutes(apiRoute.Group("/v1"))

	safe.GoSafeWithCtx("http-server", baseCtx, func(ctx context.Context) {
		port := cfg.Port
		slog.InfoContext(ctx, "starting http server", "port", port)
		if err := engine.Run(":" + strconv.Itoa(port)); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.ErrorContext(ctx, "failed to start http server", "error", err)
			return
		}
	})

	<-baseCtx.Done()
	slog.InfoContext(baseCtx, "shutting down server")
}
