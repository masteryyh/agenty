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

	"github.com/masteryyh/agenty/pkg/config"
	"github.com/masteryyh/agenty/pkg/utils/logger"
	"github.com/masteryyh/agenty/pkg/version"
	"github.com/spf13/cobra"
)

var (
	serverPort   int
	databasePath string
	debugMode    bool
	showVersion  bool
	rootCmd      = &cobra.Command{
		Use:   "agenty",
		Short: "Agenty - An AI agent application",
		Long: `Agenty is an AI agent application with tool calling, 
agentic looping and skills usage capabilities.`,
		SilenceUsage: true,
		Args: func(cmd *cobra.Command, args []string) error {
			return withExitCode(cobra.NoArgs(cmd, args), 2)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if showVersion {
				fmt.Fprintln(cmd.OutOrStdout(), version.Current())
				return nil
			}

			cfg, err := config.NewServerConfig(serverPort, databasePath, debugMode)
			if err != nil {
				return withExitCode(fmt.Errorf("invalid server options: %w", err), 2)
			}
			if err := logger.Init(cfg.Debug, ""); err != nil {
				return fmt.Errorf("failed to initialize logger: %w", err)
			}
			defer logger.Close()
			return startServer(cfg)
		},
	}
)

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.Flags().IntVar(&serverPort, "port", config.DefaultServerPort, "HTTP server port")
	rootCmd.Flags().StringVar(&databasePath, "db", config.DefaultSQLitePath, "SQLite database path")
	rootCmd.Flags().BoolVar(&debugMode, "debug", false, "enable debug logging")
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "show version")
}

func Execute() error {
	return rootCmd.Execute()
}
