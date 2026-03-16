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
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"strings"

	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/chat/tools"
	"github.com/masteryyh/agenty/pkg/chat/tools/builtin"
	"github.com/masteryyh/agenty/pkg/config"
	"github.com/masteryyh/agenty/pkg/conn"
	"github.com/masteryyh/agenty/pkg/consts"
	mcppkg "github.com/masteryyh/agenty/pkg/mcp"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/logger"
	"github.com/masteryyh/agenty/pkg/utils/signal"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	cfgFile    string
	daemonMode bool
	rootCmd    = &cobra.Command{
		Use:   "agenty",
		Short: "Agenty - An AI agent application",
		Long: `Agenty is an AI agent application with tool calling, 
agentic looping and skills usage capabilities.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.Init(cfgFile); err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}
			cfg := config.GetConfigManager().GetConfig()
			cfg.Daemon = daemonMode
			if err := config.GetConfigManager().Validate(); err != nil {
				return fmt.Errorf("invalid configuration: %w", err)
			}

			if err := logger.Init(cfg.Daemon, cfg.Debug, ""); err != nil {
				return fmt.Errorf("failed to initialize logger: %w", err)
			}
			defer logger.Close()

			if daemonMode {
				return startDaemon()
			}

			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return fmt.Errorf("must be run in an interactive terminal")
			}

			if cfg.IsRemoteMode() {
				b := backend.NewRemoteBackend(cfg.Server.URL, cfg.Server.Username, cfg.Server.Password)
				return startChat(b)
			}

			return startLocalMode()
		},
	}
)

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./agenty.yaml)")
	rootCmd.PersistentFlags().BoolVar(&daemonMode, "daemon", false, "run as backend HTTP service")

	pterm.DefaultInteractiveSelect.Selector = "❯"
	pterm.DefaultInteractiveSelect.MaxHeight = 8
}

func startLocalMode() error {
	cfg := config.GetConfigManager().GetConfig()

	baseCtx, cancel := signal.SetupContext()
	defer cancel()

	slog.InfoContext(baseCtx, "initializing database connection...")
	if err := conn.InitDB(baseCtx, cfg.DB); err != nil {
		return fmt.Errorf("failed to initialize database connection: %w", err)
	}

	slog.InfoContext(baseCtx, "registering built-in tools...")
	registry := tools.GetRegistry()
	builtin.RegisterAll(registry)

	slog.InfoContext(baseCtx, "initializing MCP manager...")
	mcpManager := mcppkg.InitManager(baseCtx, registry)
	mcpManager.Start()
	defer mcpManager.Close()

	b := backend.NewLocalBackend()
	return startChat(b)
}

func Execute() error {
	return rootCmd.Execute()
}

func showBanner() {
	index := rand.IntN(len(consts.ASCIIArts))
	banner := strings.Trim(consts.ASCIIArts[index], "\n")
	fmt.Println(pterm.FgLightCyan.Sprint(banner))
	fmt.Println(pterm.FgGray.Sprint("  AI Agent Platform"))
	fmt.Println()
}

func printSection(title string) {
	fmt.Printf("\n  %s\n  %s\n\n", pterm.Bold.Sprint(title), pterm.FgGray.Sprint(strings.Repeat("─", 56)))
}

func modelDisplayName(m models.ModelDto) string {
	if m.Provider != nil {
		return m.Provider.Name + "/" + m.Name
	}
	return m.Name
}
