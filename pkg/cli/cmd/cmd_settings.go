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

	"github.com/charmbracelet/huh"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/models"
)

var webSearchProviderOptions = []string{"disabled", "tavily", "brave", "firecrawl"}

func handleSettingsCmd(b backend.Backend, bridge *UIBridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	settings, err := b.GetSystemSettings()
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to get settings: %w", err)
	}

	if len(args) == 0 {
		finalResult := CommandResult{Handled: true}
		for {
			menuItems := []string{
				"System settings",
				"Agent models  " + styleGray.Render("(primary / fallback order)"),
				"Embedding model  " + styleGray.Render("(active model for memory search)"),
			}
			res, err := bridge.ShowList("Settings", menuItems, "↑/↓ navigate  ·  Enter select  ·  Esc back")
			if err != nil {
				return finalResult, err
			}
			switch res.Action {
			case ListActionSelect:
				switch res.Index {
				case 0:
					if err := doEditSettings(b, bridge, settings); err != nil && !errors.Is(err, ErrCancelled) {
						bridge.Error("Failed to update settings: %v", err)
					}
					continue
				case 1:
					result, err := doEditAgentModels(b, bridge, agentID)
					if err != nil && !errors.Is(err, ErrCancelled) {
						bridge.Error("Failed to update agent models: %v", err)
					}
					if result.NewModelID != uuid.Nil {
						finalResult = result
					}
					continue
				case 2:
					if err := doEditEmbeddingModel(b, bridge, settings); err != nil && !errors.Is(err, ErrCancelled) {
						bridge.Error("Failed to update embedding model: %v", err)
					}
					continue
				}
			case ListActionCancel:
				return finalResult, nil
			}
		}
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

func printSettings(bridge *UIBridge, settings *models.SystemSettingsDto) {
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
		providerStr = styleGreen.Render(string(settings.WebSearchProvider))
	}
	sb.WriteString(renderKV("Provider", providerStr, 24))

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

func doEditSettings(b backend.Backend, bridge *UIBridge, settings *models.SystemSettingsDto) error {
	defaultProviderIdx := 0
	for i, opt := range webSearchProviderOptions {
		if opt == string(settings.WebSearchProvider) {
			defaultProviderIdx = i
			break
		}
	}

	providerOpts := make([]huh.Option[string], len(webSearchProviderOptions))
	for i, p := range webSearchProviderOptions {
		providerOpts[i] = huh.NewOption(p, p)
	}

	selectedProvider := webSearchProviderOptions[defaultProviderIdx]
	var braveKey, tavilyKey, firecrawlKey, firecrawlBaseURL string

	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Web Search Provider").Options(providerOpts...).Value(&selectedProvider),
		huh.NewInput().Title("Brave API Key").Placeholder("leave blank to keep").Value(&braveKey),
		huh.NewInput().Title("Tavily API Key").Placeholder("leave blank to keep").Value(&tavilyKey),
		huh.NewInput().Title("Firecrawl API Key").Placeholder("leave blank to keep").Value(&firecrawlKey),
		huh.NewInput().Title("Firecrawl Base URL").Placeholder("leave blank for default (https://api.firecrawl.dev)").Value(&firecrawlBaseURL),
	))

	submitted, err := bridge.ShowHuhForm(form)
	if err != nil {
		return err
	}
	if !submitted {
		return ErrCancelled
	}

	provider := models.WebSearchProvider(selectedProvider)
	dto := &models.UpdateSystemSettingsDto{
		WebSearchProvider: &provider,
	}

	if strings.TrimSpace(braveKey) != "" {
		dto.BraveAPIKey = &braveKey
	}
	if strings.TrimSpace(tavilyKey) != "" {
		dto.TavilyAPIKey = &tavilyKey
	}
	if strings.TrimSpace(firecrawlKey) != "" {
		dto.FirecrawlAPIKey = &firecrawlKey
	}
	if strings.TrimSpace(firecrawlBaseURL) != "" {
		dto.FirecrawlBaseURL = &firecrawlBaseURL
	}

	if _, err := b.UpdateSystemSettings(dto); err != nil {
		return err
	}
	bridge.Success("Settings updated successfully")
	return nil
}

func doEditAgentModels(b backend.Backend, bridge *UIBridge, agentID uuid.UUID) (CommandResult, error) {
	agent, err := b.GetAgent(agentID)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to get agent: %w", err)
	}

	currentIDs := make([]uuid.UUID, len(agent.Models))
	for i, m := range agent.Models {
		currentIDs[i] = m.ID
	}

	modelIDs, err := selectAgentModels(b, bridge, currentIDs)
	if errors.Is(err, ErrCancelled) {
		return CommandResult{Handled: true}, nil
	}
	if err != nil {
		return CommandResult{Handled: true}, err
	}

	if err := b.UpdateAgent(agentID, &models.UpdateAgentDto{ModelIDs: &modelIDs}); err != nil {
		return CommandResult{Handled: true}, err
	}
	bridge.Success("Agent models updated")

	if len(modelIDs) == 0 {
		return CommandResult{Handled: true}, nil
	}

	updated, err := b.GetAgent(agentID)
	if err != nil || len(updated.Models) == 0 {
		return CommandResult{Handled: true}, nil
	}
	primary := updated.Models[0]
	return CommandResult{Handled: true, NewModelID: primary.ID, NewModelName: modelDisplayName(primary)}, nil
}

func doEditEmbeddingModel(b backend.Backend, bridge *UIBridge, settings *models.SystemSettingsDto) error {
	allModels, err := b.ListModels(1, 500)
	if err != nil {
		return fmt.Errorf("failed to list models: %w", err)
	}

	var embeddingModels []models.ModelDto
	for _, m := range allModels.Data {
		if m.EmbeddingModel && m.Provider != nil && m.Provider.APIKeyCensored != "<not set>" {
			embeddingModels = append(embeddingModels, m)
		}
	}
	if len(embeddingModels) == 0 {
		bridge.Warning("No embedding models found. Create one via /model first.")
		return nil
	}

	labels := make([]string, len(embeddingModels))
	for i, m := range embeddingModels {
		label := m.Provider.Name + "/" + m.Name + " (" + m.Code + ")"
		if settings.EmbeddingModelID != nil && *settings.EmbeddingModelID == m.ID {
			label += "  " + styleGreen.Render("● active")
		}
		labels[i] = label
	}

	res, err := bridge.ShowList("Select Embedding Model", labels, "↑/↓ navigate  ·  Enter select  ·  Esc back")
	if err != nil {
		return err
	}
	if res.Action != ListActionSelect {
		return ErrCancelled
	}

	chosen := embeddingModels[res.Index]

	if settings.EmbeddingModelID != nil && *settings.EmbeddingModelID == chosen.ID {
		bridge.Info("'%s/%s' is already the active embedding model.", chosen.Provider.Name, chosen.Name)
		return nil
	}

	if settings.EmbeddingModelID != nil {
		bridge.Error("⚠  WARNING: Switching the embedding model will INVALIDATE all existing vector data.")
		bridge.Error("   ALL knowledge base items and memories must be fully re-embedded from scratch.")
		bridge.Error("   This process may take a long time and incur significant API costs.")
		bridge.Warning("   Type \"yes\" at the next prompt to confirm you understand and wish to continue.")
		confirmed, err := bridge.ShowConfirm("I understand — switch the active embedding model and re-generate all vector data?")
		if err != nil {
			return err
		}
		if !confirmed {
			return ErrCancelled
		}
	}

	if _, err := b.UpdateSystemSettings(&models.UpdateSystemSettingsDto{EmbeddingModelID: &chosen.ID}); err != nil {
		return fmt.Errorf("failed to update embedding model: %w", err)
	}
	bridge.Success("Active embedding model set to: %s/%s", chosen.Provider.Name, chosen.Name)
	if settings.EmbeddingModelID != nil {
		bridge.Info("Re-embedding in progress in background.")
	}
	return nil
}
