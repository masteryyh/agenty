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
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/cli/ui"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/pterm/pterm"
)

var webSearchProviderOptions = []string{"disabled", "tavily", "brave", "firecrawl"}

func handleSettingsCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	settings, err := b.GetSystemSettings()
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to get settings: %w", err)
	}

	if len(args) == 0 {
		if err := doEditSettings(b, settings); err != nil && !errors.Is(err, ui.ErrCancelled) {
			pterm.Error.Printf("Failed to update settings: %v\n", err)
		}
		return CommandResult{Handled: true}, nil
	}

	switch args[0] {
	case "show":
		printSettings(settings)
	case "edit":
		if err := doEditSettings(b, settings); err != nil && !errors.Is(err, ui.ErrCancelled) {
			pterm.Error.Printf("Failed to update settings: %v\n", err)
		}
	default:
		pterm.Warning.Printf("Unknown subcommand: %s\n", args[0])
		pterm.Info.Println("Usage: /settings [show|edit]")
	}

	return CommandResult{Handled: true}, nil
}

func printSettings(settings *models.SystemSettingsDto) {
	fmt.Println()
	fmt.Printf("  %s\n  %s\n\n", pterm.Bold.Sprint("System Settings"), pterm.FgGray.Sprint(strings.Repeat("─", 56)))

	embeddingModel := pterm.FgGray.Sprint("not set")
	if settings.EmbeddingModelID != nil {
		embeddingModel = pterm.FgCyan.Sprint(settings.EmbeddingModelID.String())
	}
	compressionModel := pterm.FgGray.Sprint("not set")
	if settings.ContextCompressionModelID != nil {
		compressionModel = pterm.FgCyan.Sprint(settings.ContextCompressionModelID.String())
	}

	fmt.Printf("  %-24s %s\n", pterm.FgGray.Sprint("Embedding Model"), embeddingModel)
	fmt.Printf("  %-24s %s\n", pterm.FgGray.Sprint("Compression Model"), compressionModel)

	fmt.Println()
	fmt.Printf("  %s\n  %s\n\n", pterm.Bold.Sprint("Web Search"), pterm.FgGray.Sprint(strings.Repeat("─", 56)))

	providerStr := pterm.FgGray.Sprint("disabled")
	if settings.WebSearchProvider != "" && settings.WebSearchProvider != models.WebSearchProviderDisabled {
		providerStr = pterm.FgGreen.Sprint(string(settings.WebSearchProvider))
	}
	fmt.Printf("  %-24s %s\n", pterm.FgGray.Sprint("Provider"), providerStr)

	printAPIKeyField := func(label, val string) {
		display := pterm.FgGray.Sprint("not set")
		if val != "" {
			display = pterm.FgYellow.Sprint(val)
		}
		fmt.Printf("  %-24s %s\n", pterm.FgGray.Sprint(label), display)
	}

	printAPIKeyField("Brave API Key", settings.BraveAPIKey)
	printAPIKeyField("Tavily API Key", settings.TavilyAPIKey)
	printAPIKeyField("Firecrawl API Key", settings.FirecrawlAPIKey)
	if settings.FirecrawlBaseURL != "" {
		fmt.Printf("  %-24s %s\n", pterm.FgGray.Sprint("Firecrawl Base URL"), pterm.FgGray.Sprint(settings.FirecrawlBaseURL))
	}
	fmt.Println()
}

func doEditSettings(b backend.Backend, settings *models.SystemSettingsDto) error {
	defaultProviderIdx := 0
	for i, opt := range webSearchProviderOptions {
		if opt == string(settings.WebSearchProvider) {
			defaultProviderIdx = i
			break
		}
	}

	fields := []*ui.FormField{
		ui.SelectField("Web Search Provider", webSearchProviderOptions, defaultProviderIdx),
		ui.TextField("Brave API Key", "", false),
		ui.TextField("Tavily API Key", "", false),
		ui.TextField("Firecrawl API Key", "", false),
		ui.TextField("Firecrawl Base URL", settings.FirecrawlBaseURL, false),
	}

	fields[1].Placeholder = "leave blank to keep"
	fields[2].Placeholder = "leave blank to keep"
	fields[3].Placeholder = "leave blank to keep"
	fields[4].Placeholder = "leave blank for default (https://api.firecrawl.dev)"

	submitted, err := ui.ShowForm("System Settings", fields)
	if err != nil {
		return err
	}
	if !submitted {
		return ui.ErrCancelled
	}

	provider := models.WebSearchProvider(webSearchProviderOptions[fields[0].SelIdx])
	dto := &models.UpdateSystemSettingsDto{
		WebSearchProvider: &provider,
	}

	if v := fields[1].Value; v != "" {
		dto.BraveAPIKey = &v
	}
	if v := fields[2].Value; v != "" {
		dto.TavilyAPIKey = &v
	}
	if v := fields[3].Value; v != "" {
		dto.FirecrawlAPIKey = &v
	}
	if v := fields[4].Value; v != "" {
		dto.FirecrawlBaseURL = &v
	}

	if _, err := b.UpdateSystemSettings(dto); err != nil {
		return err
	}
	pterm.Success.Println("Settings updated successfully")
	return nil
}
