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
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/muesli/reflow/truncate"
)

var providerTypeOptions = []string{"openai", "anthropic", "gemini", "kimi", "bigmodel", "qwen", "deepseek"}

var providerDefaultBaseURLs = map[string]string{
	"openai":        "https://api.openai.com/v1",
	"openai-legacy": "https://api.openai.com/v1",
	"anthropic":     "https://api.anthropic.com",
	"gemini":        "https://generativelanguage.googleapis.com/v1beta",
	"kimi":          "https://api.moonshot.cn/v1",
	"bigmodel":      "https://open.bigmodel.cn/api/paas/v4",
	"qwen":          "https://dashscope.aliyuncs.com/compatible-mode/v1",
	"deepseek":      "https://api.deepseek.com",
}

func providerLabel(p models.ModelProviderDto) string {
	return fmt.Sprintf("%s (%s)", p.Name, string(p.Type))
}

func handleProviderCmd(b backend.Backend, bridge *UIBridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	for {
		result, err := b.ListProviders(1, 100)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to list providers: %w", err)
		}

		if len(result.Data) == 0 {
			bridge.Warning("No providers found")
			res, err := bridge.ShowList("Providers", []string{"(no providers)"}, "a add  ·  Esc back")
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if res.Action == ListActionAdd {
				if err := doCreateProvider(b, bridge); err != nil && !errors.Is(err, ErrCancelled) {
					bridge.Error("Failed to create provider: %v", err)
				}
				continue
			}
			return CommandResult{Handled: true}, nil
		}

		items := providerTableRows(result.Data)
		res, err := bridge.ShowListWithCursorAndActions("Providers", items, listHints, 0, validateProviderListAction(result.Data), providerDeleteConfirm(result.Data), providerTableSubtitle())
		if err != nil {
			return CommandResult{Handled: true}, err
		}

		switch res.Action {
		case ListActionSelect:
			if err := doUpdateProvider(b, bridge, result.Data[res.Index]); err != nil && !errors.Is(err, ErrCancelled) {
				bridge.Error("Failed to update provider: %v", err)
			}
			continue

		case ListActionAdd:
			if err := doCreateProvider(b, bridge); err != nil && !errors.Is(err, ErrCancelled) {
				bridge.Error("Failed to create provider: %v", err)
			}
			continue

		case ListActionEdit:
			if err := doUpdateProvider(b, bridge, result.Data[res.Index]); err != nil && !errors.Is(err, ErrCancelled) {
				bridge.Error("Failed to update provider: %v", err)
			}
			continue

		case ListActionDelete:
			target := result.Data[res.Index]
			if err := b.DeleteProvider(target.ID, true); err != nil {
				bridge.Error("Failed to delete provider: %v", err)
			} else {
				bridge.Success("Provider deleted: %s", target.Name)
			}
			continue

		case ListActionCancel:
			return CommandResult{Handled: true}, nil
		}
	}
}

func providerDeleteConfirm(providerList []models.ModelProviderDto) func(idx int) string {
	return func(idx int) string {
		if idx < 0 || idx >= len(providerList) {
			return ""
		}
		target := providerList[idx]
		if target.IsPreset {
			return ""
		}
		return fmt.Sprintf("Delete provider '%s' and all its models?", target.Name)
	}
}

func validateProviderListAction(providerList []models.ModelProviderDto) func(action ListAction, idx int) error {
	return func(action ListAction, idx int) error {
		if idx < 0 || idx >= len(providerList) {
			return nil
		}
		target := providerList[idx]
		if !target.IsPreset {
			return nil
		}
		switch action {
		case ListActionSelect, ListActionEdit:
			return fmt.Errorf("'%s' is a preset provider and cannot be modified.", target.Name)
		case ListActionDelete:
			return fmt.Errorf("'%s' is a preset provider and cannot be deleted.", target.Name)
		default:
			return nil
		}
	}
}

func providerTableRows(providers []models.ModelProviderDto) []string {
	rows := make([]string, len(providers))
	for i, p := range providers {
		rows[i] = providerTableRow(p)
	}
	return rows
}

func providerTableSubtitle() string {
	header := providerTableFormat("Name", "Base URL", "API Key")
	return "  " + header + "\n  " + strings.Repeat("─", lipgloss.Width(header))
}

func providerTableRow(p models.ModelProviderDto) string {
	return providerTableFormat(p.Name, p.BaseURL, p.APIKeyCensored)
}

func providerTableFormat(name, baseURL, apiKey string) string {
	return fmt.Sprintf("%s  %s  %s",
		providerTableCell(name, 18),
		providerTableCell(baseURL, 34),
		providerTableCell(apiKey, 18),
	)
}

func providerTableCell(s string, width int) string {
	if s == "" {
		return "-"
	}
	value := truncate.StringWithTail(s, uint(width), "...")
	if padding := width - lipgloss.Width(value); padding > 0 {
		return value + strings.Repeat(" ", padding)
	}
	return value
}

func doCreateProvider(b backend.Backend, bridge *UIBridge) error {
	typeOpts := make([]huh.Option[string], len(providerTypeOptions))
	for i, t := range providerTypeOptions {
		typeOpts[i] = huh.NewOption(t, t)
	}

	var name, selectedType, baseURL, apiKey string
	selectedType = providerTypeOptions[0]
	baseURL = providerDefaultBaseURLs[selectedType]

	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Name").Value(&name),
		huh.NewSelect[string]().Title("Type").Options(typeOpts...).Value(&selectedType),
		huh.NewInput().Title("Base URL").Value(&baseURL),
		huh.NewInput().Title("API key").Value(&apiKey),
	))

	submitted, err := bridge.ShowValidatedHuhForm(form, func() error {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("Name is required.")
		}
		if strings.TrimSpace(baseURL) == "" {
			return fmt.Errorf("Base URL is required.")
		}
		if strings.TrimSpace(apiKey) == "" {
			return fmt.Errorf("API key is required.")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !submitted {
		return ErrCancelled
	}

	provider, err := b.CreateProvider(&models.CreateModelProviderDto{
		Name:    strings.TrimSpace(name),
		Type:    models.APIType(selectedType),
		BaseURL: strings.TrimSpace(baseURL),
		APIKey:  apiKey,
	})
	if err != nil {
		return err
	}
	bridge.Success("Provider created: %s", provider.Name)
	return nil
}

func doUpdateProvider(b backend.Backend, bridge *UIBridge, target models.ModelProviderDto) error {
	defaultTypeIdx := 0
	for i, opt := range providerTypeOptions {
		if opt == string(target.Type) {
			defaultTypeIdx = i
			break
		}
	}

	typeOpts := make([]huh.Option[string], len(providerTypeOptions))
	for i, t := range providerTypeOptions {
		typeOpts[i] = huh.NewOption(t, t)
	}

	newName := target.Name
	selectedType := providerTypeOptions[defaultTypeIdx]
	newBaseURL := target.BaseURL
	var newAPIKey string

	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Name").Value(&newName),
		huh.NewSelect[string]().Title("Type").Options(typeOpts...).Value(&selectedType),
		huh.NewInput().Title("Base URL").Value(&newBaseURL),
		huh.NewInput().Title("API key").Placeholder("leave blank to keep").Value(&newAPIKey),
	))

	submitted, err := bridge.ShowValidatedHuhForm(form, func() error {
		if strings.TrimSpace(newName) == "" {
			return fmt.Errorf("Name is required.")
		}
		if strings.TrimSpace(newBaseURL) == "" {
			return fmt.Errorf("Base URL is required.")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !submitted {
		return ErrCancelled
	}

	newName = strings.TrimSpace(newName)
	newBaseURL = strings.TrimSpace(newBaseURL)

	if target.Name == newName && string(target.Type) == selectedType && target.BaseURL == newBaseURL && newAPIKey == "" {
		bridge.Info("No changes detected, skipping update")
		return nil
	}

	updated, err := b.UpdateProvider(target.ID, &models.UpdateModelProviderDto{
		Name:    newName,
		Type:    models.APIType(selectedType),
		BaseURL: newBaseURL,
		APIKey:  newAPIKey,
	})
	if err != nil {
		return err
	}
	bridge.Success("Provider updated: %s", updated.Name)
	return nil
}
