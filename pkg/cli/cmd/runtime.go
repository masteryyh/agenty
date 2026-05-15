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
	"fmt"
	"log/slog"

	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/chat/sessionhooks"
	"github.com/masteryyh/agenty/pkg/config"
	"github.com/masteryyh/agenty/pkg/conn"
	mcppkg "github.com/masteryyh/agenty/pkg/mcp"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/tools"
	"github.com/masteryyh/agenty/pkg/tools/builtin"
	"github.com/masteryyh/agenty/pkg/utils/logger"
	"github.com/masteryyh/agenty/pkg/utils/signal"
	"github.com/spf13/cobra"
)

type Runtime struct {
	Backend backend.Backend
	Local   bool
	Close   func()
}

func initCommandEnvironment(isDaemon bool) (*config.AppConfig, func(), error) {
	if err := config.Init(cfgFile); err != nil {
		return nil, nil, withExitCode(fmt.Errorf("failed to load configuration: %w", err), 3)
	}

	cfg := config.GetConfigManager().GetConfig()
	cfg.Daemon = isDaemon

	if err := config.GetConfigManager().Validate(); err != nil {
		return nil, nil, withExitCode(fmt.Errorf("invalid configuration: %w", err), 3)
	}

	if err := logger.Init(cfg.Daemon, cfg.Debug, ""); err != nil {
		return nil, nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	return cfg, func() {
		logger.Close()
	}, nil
}

func initRuntime(_ context.Context, needsMCP bool, needsSkills bool) (*Runtime, error) {
	cfg, closeLogger, err := initCommandEnvironment(false)
	if err != nil {
		return nil, err
	}

	if cfg.IsRemoteMode() {
		b, err := backend.NewRemoteBackend(cfg.Server.URL, cfg.Server.Username, cfg.Server.Password)
		if err != nil {
			closeLogger()
			return nil, withExitCode(err, 4)
		}
		return &Runtime{
			Backend: b,
			Local:   false,
			Close:   closeLogger,
		}, nil
	}

	return initLocalRuntimeFromConfig(cfg, closeLogger, needsMCP, needsSkills)
}

func initLocalRuntime(_ context.Context, needsMCP bool, needsSkills bool) (*Runtime, *config.AppConfig, error) {
	cfg, closeLogger, err := initCommandEnvironment(false)
	if err != nil {
		return nil, nil, err
	}
	if cfg.IsRemoteMode() {
		closeLogger()
		return nil, nil, withExitCode(fmt.Errorf("init only supports local mode; remove server.url or use a local config"), 3)
	}
	runtime, err := initLocalRuntimeFromConfig(cfg, closeLogger, needsMCP, needsSkills)
	if err != nil {
		return nil, nil, err
	}
	return runtime, cfg, nil
}

func initLocalRuntimeFromConfig(cfg *config.AppConfig, closeLogger func(), needsMCP bool, needsSkills bool) (*Runtime, error) {
	baseCtx, cancel := signal.SetupContext()

	if err := conn.InitDB(baseCtx, cfg.DB, cfg.Debug); err != nil {
		cancel()
		closeLogger()
		return nil, fmt.Errorf("failed to initialize database connection: %w", err)
	}

	registry := tools.GetRegistry()
	builtin.RegisterAll(registry)
	sessionhooks.RegisterAll()

	var mcpManager *mcppkg.MCPManager
	if needsMCP {
		mcpManager = mcppkg.InitManager(baseCtx, registry)
		mcpManager.Start()
	}

	var skillSvc *services.SkillService
	if needsSkills {
		skillSvc = services.GetSkillService()
		if err := skillSvc.Initialize(baseCtx); err != nil {
			slog.WarnContext(baseCtx, "skill service initialization failed", "error", err)
		}
	}

	return &Runtime{
		Backend: backend.NewLocalBackend(),
		Local:   true,
		Close: func() {
			if skillSvc != nil {
				skillSvc.Shutdown()
			}
			if mcpManager != nil {
				mcpManager.Close()
			}
			cancel()
			closeLogger()
		},
	}, nil
}

func runWithRuntime(cmd *cobra.Command, needsMCP bool, needsSkills bool, run func(runtime *Runtime) error) error {
	runtime, err := initRuntime(cmd.Context(), needsMCP, needsSkills)
	if err != nil {
		return err
	}
	defer runtime.Close()
	return run(runtime)
}
