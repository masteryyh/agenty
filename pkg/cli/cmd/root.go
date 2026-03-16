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
	"math/rand/v2"
	"os"
	"strings"

	"github.com/masteryyh/agenty/pkg/cli/api"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	cfgFile string
	baseURL string
	rootCmd = &cobra.Command{
		Use:   "agenty",
		Short: "Agenty - An AI agent application",
		Long: `Agenty is an AI agent application with tool calling, 
agentic looping and skills usage capabilities.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return fmt.Errorf("must be run in an interactive terminal")
			}
			return startChat()
		},
	}
)

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
	rootCmd.PersistentFlags().StringVar(&baseURL, "url", "", "Backend API base URL (overrides config file)")

	pterm.DefaultInteractiveSelect.Selector = "❯"
	pterm.DefaultInteractiveSelect.MaxHeight = 8
}

func GetBaseURL() string {
	if baseURL != "" {
		return baseURL
	}

	cfg := GetCLIConfig()
	if cfg.BaseURL != "" {
		return cfg.BaseURL
	}

	return "http://localhost:8080"
}

func GetClient() *api.Client {
	cfg := GetCLIConfig()
	url := GetBaseURL()

	if cfg.Username != "" && cfg.Password != "" {
		return api.NewClientWithAuth(url, cfg.Username, cfg.Password)
	}

	return api.NewClient(url)
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
