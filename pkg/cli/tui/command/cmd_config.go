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

func handleConfigCmd(b backend.Backend, bridge Bridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	config, err := b.GetSystemConfig()
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to get config: %w", err)
	}

	if len(args) == 0 {
		if err := doEditConfig(b, bridge, config); err != nil && !errors.Is(err, ErrCancelled) {
			bridge.Error("Failed to update config: %v", err)
		}
		return CommandResult{Handled: true}, nil
	}

	switch args[0] {
	case "show":
		printConfig(bridge, config)
	case "edit":
		if err := doEditConfig(b, bridge, config); err != nil && !errors.Is(err, ErrCancelled) {
			bridge.Error("Failed to update config: %v", err)
		}
	default:
		bridge.Warning("Unknown subcommand: %s", args[0])
		bridge.Info("Usage: /config [show|edit]")
	}

	return CommandResult{Handled: true}, nil
}

func printConfig(bridge Bridge, config *models.SystemConfigDto) {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(renderSectionHeader("System Config"))

	embeddingModel := styleGray.Render("not set")
	if config.EmbeddingModelID != nil {
		embeddingModel = styleCyan.Render(config.EmbeddingModelID.String())
	}
	compressionModel := styleGray.Render("not set")
	if config.ContextCompressionModelID != nil {
		compressionModel = styleCyan.Render(config.ContextCompressionModelID.String())
	}

	sb.WriteString(renderKV("Embedding Model", embeddingModel, 24))
	sb.WriteString(renderKV("Compression Model", compressionModel, 24))

	sb.WriteString("\n")
	sb.WriteString(renderSectionHeader("Web Search"))

	providerStr := styleGray.Render("disabled")
	if config.WebSearchProvider != "" && config.WebSearchProvider != models.WebSearchProviderDisabled {
		providerStr = styleGreen.Render(webSearchProviderDisplayName(config.WebSearchProvider))
	}
	sb.WriteString(renderKV("Provider", providerStr, 24))
	sb.WriteString(renderKV("Configured", renderConfiguredWebSearchProviders(config), 24))

	writeAPIKeyField := func(label, val string) {
		display := styleGray.Render("not set")
		if val != "" {
			display = styleYellow.Render(val)
		}
		sb.WriteString(renderKV(label, display, 24))
	}

	writeAPIKeyField("Brave API Key", config.BraveAPIKey)
	writeAPIKeyField("Tavily API Key", config.TavilyAPIKey)
	writeAPIKeyField("Firecrawl API Key", config.FirecrawlAPIKey)
	if config.FirecrawlBaseURL != "" {
		sb.WriteString(renderKV("Firecrawl Base URL", styleGray.Render(config.FirecrawlBaseURL), 24))
	}

	bridge.Print(sb.String())
}

func doEditConfig(b backend.Backend, bridge Bridge, config *models.SystemConfigDto) error {
	return bridge.ShowConfigEditor(b, config)
}

func renderConfiguredWebSearchProviders(config *models.SystemConfigDto) string {
	if len(config.ConfiguredWebSearchProviders) == 0 {
		return styleGray.Render("none")
	}

	names := make([]string, len(config.ConfiguredWebSearchProviders))
	for i, provider := range config.ConfiguredWebSearchProviders {
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
