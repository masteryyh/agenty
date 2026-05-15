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
	"os"

	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/version"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	cfgFile       string
	daemonMode    bool
	showVersion   bool
	outputJSON    bool
	quietOutput   bool
	noColorOutput bool
	page          int
	pageSize      int
	rootCmd       = &cobra.Command{
		Use:   "agenty",
		Short: "Agenty - An AI agent application",
		Long: `Agenty is an AI agent application with tool calling, 
agentic looping and skills usage capabilities.`,
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if daemonMode && cmd.Parent() != nil {
				return withExitCode(fmt.Errorf("--daemon can only be used without subcommands"), 2)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				fmt.Fprintln(cmd.OutOrStdout(), version.Current())
				return nil
			}

			if daemonMode {
				_, closeLogger, err := initCommandEnvironment(true)
				if err != nil {
					return err
				}
				defer closeLogger()
				return startDaemon()
			}

			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return withExitCode(fmt.Errorf("must be run in an interactive terminal"), 2)
			}

			runtime, err := initRuntime(cmd.Context(), true, true)
			if err != nil {
				return err
			}
			defer runtime.Close()
			return startChat(runtime.Backend, runtime.Local)
		},
	}
)

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.agenty/config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&daemonMode, "daemon", false, "run as backend HTTP service")
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "show version")
	rootCmd.PersistentFlags().BoolVar(&outputJSON, "json", false, "output JSON")
	rootCmd.PersistentFlags().BoolVar(&quietOutput, "quiet", false, "suppress non-essential output")
	rootCmd.PersistentFlags().BoolVar(&noColorOutput, "no-color", false, "disable color output")
	rootCmd.PersistentFlags().IntVar(&page, "page", 1, "page number for list commands")
	rootCmd.PersistentFlags().IntVar(&pageSize, "page-size", 50, "page size for list commands")
	rootCmd.AddCommand(newChatCmd())
	rootCmd.AddCommand(newAgentCmd())
	rootCmd.AddCommand(newProviderCmd())
	rootCmd.AddCommand(newModelCmd())
	rootCmd.AddCommand(newSettingsCmd())
	rootCmd.AddCommand(newSessionCmd())
	rootCmd.AddCommand(newMemoryCmd())
	rootCmd.AddCommand(newSkillCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newMCPCmd())
}

func Execute() error {
	return rootCmd.Execute()
}

func modelDisplayName(m models.ModelDto) string {
	if m.Provider != nil {
		return m.Provider.Name + "/" + m.Name
	}
	return m.Name
}
