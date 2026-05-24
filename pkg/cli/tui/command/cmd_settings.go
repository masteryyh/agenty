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

package command

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/models"
)

var webSearchProviders = []models.WebSearchProvider{
	models.WebSearchProviderTavily,
	models.WebSearchProviderBrave,
	models.WebSearchProviderFirecrawl,
}

func handleSettingsCmd(b backend.Backend, bridge Bridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	settings, err := b.GetSystemSettings()
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to get settings: %w", err)
	}

	if len(args) == 0 {
		if err := doEditSettings(b, bridge, settings); err != nil && !errors.Is(err, ErrCancelled) {
			bridge.Error("Failed to update settings: %v", err)
		}
		return CommandResult{Handled: true}, nil
	}

	switch args[0] {
	case "show":
		printSettings(bridge, settings)
	case "edit":
		if err := doEditSettings(b, bridge, settings); err != nil && !errors.Is(err, ErrCancelled) {
			bridge.Error("Failed to update settings: %v", err)
		}
	default:
		bridge.Warning("Unknown subcommand: %s", args[0])
		bridge.Info("Usage: /settings [show|edit]")
	}

	return CommandResult{Handled: true}, nil
}

func printSettings(bridge Bridge, settings *models.SystemSettingsDto) {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(renderSectionHeader("System Settings"))

	embeddingModel := styleGray.Render("not set")
	if settings.EmbeddingModelID != nil {
		embeddingModel = styleCyan.Render(settings.EmbeddingModelID.String())
	}
	compressionModel := styleGray.Render("not set")
	if settings.ContextCompressionModelID != nil {
		compressionModel = styleCyan.Render(settings.ContextCompressionModelID.String())
	}

	sb.WriteString(renderKV("Embedding Model", embeddingModel, 24))
	sb.WriteString(renderKV("Compression Model", compressionModel, 24))

	sb.WriteString("\n")
	sb.WriteString(renderSectionHeader("Web Search"))

	providerStr := styleGray.Render("disabled")
	if settings.WebSearchProvider != "" && settings.WebSearchProvider != models.WebSearchProviderDisabled {
		providerStr = styleGreen.Render(webSearchProviderDisplayName(settings.WebSearchProvider))
	}
	sb.WriteString(renderKV("Provider", providerStr, 24))
	sb.WriteString(renderKV("Configured", renderConfiguredWebSearchProviders(settings), 24))

	writeAPIKeyField := func(label, val string) {
		display := styleGray.Render("not set")
		if val != "" {
			display = styleYellow.Render(val)
		}
		sb.WriteString(renderKV(label, display, 24))
	}

	writeAPIKeyField("Brave API Key", settings.BraveAPIKey)
	writeAPIKeyField("Tavily API Key", settings.TavilyAPIKey)
	writeAPIKeyField("Firecrawl API Key", settings.FirecrawlAPIKey)
	if settings.FirecrawlBaseURL != "" {
		sb.WriteString(renderKV("Firecrawl Base URL", styleGray.Render(settings.FirecrawlBaseURL), 24))
	}

	bridge.Print(sb.String())
}

func doEditSettings(b backend.Backend, bridge Bridge, settings *models.SystemSettingsDto) error {
	return bridge.ShowSettingsEditor(b, settings)
}

func renderConfiguredWebSearchProviders(settings *models.SystemSettingsDto) string {
	if len(settings.ConfiguredWebSearchProviders) == 0 {
		return styleGray.Render("none")
	}

	names := make([]string, len(settings.ConfiguredWebSearchProviders))
	for i, provider := range settings.ConfiguredWebSearchProviders {
		names[i] = webSearchProviderDisplayName(provider)
	}
	return styleYellow.Render(strings.Join(names, ", "))
}

func webSearchProviderDisplayName(provider models.WebSearchProvider) string {
	switch provider {
	case models.WebSearchProviderTavily:
		return "Tavily"
	case models.WebSearchProviderBrave:
		return "Brave"
	case models.WebSearchProviderFirecrawl:
		return "Firecrawl"
	default:
		return string(provider)
	}
}
