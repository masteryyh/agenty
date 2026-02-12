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

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	cfgFile      string
	baseURL      string
	bannerShown  bool
	rootCmd = &cobra.Command{
		Use:   "agenty",
		Short: "Agenty - An AI agent application",
		Long: `Agenty is an AI agent application with tool calling, 
agentic looping and skills usage capabilities.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if !bannerShown {
				showBanner()
				bannerShown = true
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}
)

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
	rootCmd.PersistentFlags().StringVar(&baseURL, "url", "http://localhost:8080", "Backend API base URL")
}

func GetBaseURL() string {
	if baseURL == "" {
		return "http://localhost:8080"
	}
	return baseURL
}

func Execute() error {
	return rootCmd.Execute()
}

func showBanner() {
	banner := `
   ▄████████    ▄██████▄     ▄████████ ███▄▄▄▄       ███     ▄██   ▄   
  ███    ███   ███    ███   ███    ███ ███▀▀▀██▄ ▀█████████▄ ███   ██▄ 
  ███    ███   ███    █▀    ███    █▀  ███   ███    ▀███▀▀██ ███▄▄▄███ 
  ███    ███  ▄███          ███        ███   ███     ███   ▀ ▀▀▀▀▀▀███ 
▀███████████ ▀▀███ ████▄  ▀███████████ ███   ███     ███     ▄██   ███ 
  ███    ███   ███    ███          ███ ███   ███     ███     ███   ███ 
  ███    ███   ███    ███    ▄█    ███ ███   ███     ███     ███   ███ 
  ███    █▀    ████████▀   ▄████████▀   ▀█   █▀     ▄████▀    ▀█████▀  
`
	pterm.DefaultCenter.Println(pterm.DefaultHeader.WithBackgroundStyle(pterm.NewStyle(pterm.BgDarkGray)).WithTextStyle(pterm.NewStyle(pterm.FgLightCyan)).Sprint(banner))
	fmt.Println()
}
