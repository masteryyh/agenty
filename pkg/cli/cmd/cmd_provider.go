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

var providerTypeOptions = []string{"openai", "anthropic", "gemini", "kimi"}

var providerDefaultBaseURLs = map[string]string{
	"openai":        "https://api.openai.com/v1",
	"openai-legacy": "https://api.openai.com/v1",
	"anthropic":     "https://api.anthropic.com",
	"gemini":        "https://generativelanguage.googleapis.com/v1beta",
	"kimi":          "https://api.moonshot.cn/v1",
}

func providerLabel(p models.ModelProviderDto) string {
	return fmt.Sprintf("%s (%s)", p.Name, string(p.Type))
}

func handleProviderCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	for {
		result, err := b.ListProviders(1, 100)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to list providers: %w", err)
		}

		if len(result.Data) == 0 {
			pterm.Warning.Println("No providers found")
			res, err := ui.ShowList("Providers", []string{"(no providers)"}, "a add  ·  Esc back")
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if res.Action == ui.ListActionAdd {
				if err := doCreateProvider(b); err != nil && !errors.Is(err, ui.ErrCancelled) {
					pterm.Error.Printf("Failed to create provider: %v\n", err)
				}
				continue
			}
			return CommandResult{Handled: true}, nil
		}

		items := make([]string, len(result.Data))
		for i, p := range result.Data {
			items[i] = fmt.Sprintf("%s  %s  %s", providerLabel(p), pterm.FgGray.Sprint(p.BaseURL), pterm.FgGray.Sprint(p.APIKeyCensored))
		}

		res, err := ui.ShowList("Providers", items, listHints)
		if err != nil {
			return CommandResult{Handled: true}, err
		}

		switch res.Action {
		case ui.ListActionSelect:
			target := result.Data[res.Index]
			fmt.Println()
			fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Name"), target.Name)
			fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Type"), string(target.Type))
			fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Base URL"), target.BaseURL)
			fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("API Key"), target.APIKeyCensored)
			fmt.Println()
			continue

		case ui.ListActionAdd:
			if err := doCreateProvider(b); err != nil && !errors.Is(err, ui.ErrCancelled) {
				pterm.Error.Printf("Failed to create provider: %v\n", err)
			}
			continue

		case ui.ListActionEdit:
			if err := doUpdateProvider(b, result.Data[res.Index]); err != nil && !errors.Is(err, ui.ErrCancelled) {
				pterm.Error.Printf("Failed to update provider: %v\n", err)
			}
			continue

		case ui.ListActionDelete:
			target := result.Data[res.Index]
			confirmed, err := ui.ShowConfirm(fmt.Sprintf("Delete provider '%s' and all its models?", target.Name))
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if confirmed {
				if err := b.DeleteProvider(target.ID, true); err != nil {
					pterm.Error.Printf("Failed to delete provider: %v\n", err)
				} else {
					pterm.Success.Printf("Provider deleted: %s\n", target.Name)
				}
			}
			continue

		case ui.ListActionCancel:
			return CommandResult{Handled: true}, nil
		}
	}
}

func doCreateProvider(b backend.Backend) error {
	typeField := ui.SelectField("Type", providerTypeOptions, 0)
	urlField := ui.TextField("Base URL", providerDefaultBaseURLs[providerTypeOptions[0]], true)
	const urlFieldIdx = 2

	typeField.OnChange = func(selIdx int, inputs [][]rune, update func(int, string)) {
		typeName := providerTypeOptions[selIdx]
		newDefault, hasDefault := providerDefaultBaseURLs[typeName]
		if !hasDefault {
			return
		}
		currentURL := strings.TrimSpace(string(inputs[urlFieldIdx]))
		for _, u := range providerDefaultBaseURLs {
			if currentURL == u || currentURL == "" {
				update(urlFieldIdx, newDefault)
				return
			}
		}
	}

	fields := []*ui.FormField{
		ui.TextField("Name", "", true),
		typeField,
		urlField,
		ui.TextField("API key", "", false),
	}

	submitted, err := ui.ShowForm("Create Provider", fields)
	if err != nil {
		return err
	}
	if !submitted {
		return ui.ErrCancelled
	}

	name := fields[0].Value
	selectedType := providerTypeOptions[fields[1].SelIdx]
	baseURL := fields[2].Value
	apiKey := fields[3].Value

	provider, err := b.CreateProvider(&models.CreateModelProviderDto{
		Name:    name,
		Type:    models.APIType(selectedType),
		BaseURL: baseURL,
		APIKey:  apiKey,
	})
	if err != nil {
		return err
	}
	pterm.Success.Printf("Provider created: %s\n", provider.Name)
	return nil
}

func doUpdateProvider(b backend.Backend, target models.ModelProviderDto) error {
	defaultTypeIdx := 0
	for i, opt := range providerTypeOptions {
		if opt == string(target.Type) {
			defaultTypeIdx = i
			break
		}
	}

	apiKeyField := ui.TextField("API key", "", false)
	apiKeyField.Placeholder = "leave blank to keep"

	fields := []*ui.FormField{
		ui.TextField("Name", target.Name, true),
		ui.SelectField("Type", providerTypeOptions, defaultTypeIdx),
		ui.TextField("Base URL", target.BaseURL, true),
		apiKeyField,
	}

	submitted, err := ui.ShowForm("Update Provider", fields)
	if err != nil {
		return err
	}
	if !submitted {
		return ui.ErrCancelled
	}

	newName := fields[0].Value
	newType := providerTypeOptions[fields[1].SelIdx]
	newBaseURL := fields[2].Value
	newAPIKey := fields[3].Value

	if target.Name == newName && string(target.Type) == newType && target.BaseURL == newBaseURL && newAPIKey == "" {
		pterm.Info.Println("No changes detected, skipping update")
		return nil
	}

	updated, err := b.UpdateProvider(target.ID, &models.UpdateModelProviderDto{
		Name:    newName,
		Type:    models.APIType(newType),
		BaseURL: newBaseURL,
		APIKey:  newAPIKey,
	})
	if err != nil {
		return err
	}
	pterm.Success.Printf("Provider updated: %s\n", updated.Name)
	return nil
}
