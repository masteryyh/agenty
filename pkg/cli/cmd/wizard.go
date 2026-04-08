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

	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/cli/ui"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/pterm/pterm"
)

func runWizardIfNeeded(b backend.Backend) error {
	initialized, err := b.IsInitialized()
	if err != nil {
		pterm.Warning.Printf("Could not check initialization status: %v\n", err)
		return nil
	}
	if initialized {
		return nil
	}
	return runWizard(b)
}

func runWizard(b backend.Backend) error {
	divider := pterm.FgGray.Sprint(strings.Repeat("─", 58))

	ui.Writeln("")
	ui.Writeln("  " + pterm.Bold.Sprint("First-Time Setup"))
	ui.Writeln("  " + divider)
	ui.Writeln("")
	ui.Writeln("  " + pterm.FgGray.Sprint("Agenty comes with preset LLM providers and models."))
	ui.Writeln("  " + pterm.FgGray.Sprint("Let's configure the API keys for the ones you want to use,"))
	ui.Writeln("  " + pterm.FgGray.Sprint("and choose your default model."))
	ui.Writeln("")
	ui.Writeln("  " + pterm.FgGray.Sprintf("Press %s at any time to skip to the next step.",
		pterm.FgWhite.Sprint("Esc")))
	ui.Writeln("")

	proceed, err := ui.ShowConfirm("Start setup now?")
	ui.Writeln("")
	if err != nil || !proceed {
		ui.Writeln("  " + pterm.FgYellow.Sprint("⚠") + "  Skipping setup. The wizard will appear again on next startup.")
		ui.Writeln("     Use " + pterm.FgWhite.Sprint("/provider") + " to configure providers manually.")
		ui.Writeln("")
		return nil
	}

	providers, err := b.ListProviders(1, 100)
	if err != nil {
		return fmt.Errorf("failed to list providers: %w", err)
	}

	configuredIDs := make(map[string]bool)

	if len(providers.Data) > 0 {
		fmt.Printf("\n  %s\n  %s\n\n",
			pterm.Bold.Sprint("Configure Provider API Keys"),
			pterm.FgGray.Sprint(strings.Repeat("─", 58)))

		for i := range providers.Data {
			p := &providers.Data[i]
			ui.Writeln(fmt.Sprintf("  %s  %s",
				pterm.FgCyan.Sprintf("[%d/%d]", i+1, len(providers.Data)),
				pterm.Bold.Sprint(p.Name)+" "+pterm.FgGray.Sprint("("+string(p.Type)+")"),
			))
			ui.Writeln("  " + pterm.FgGray.Sprint("Base URL: "+p.BaseURL))
			ui.Writeln("")

			configure, err := ui.ShowConfirm("Configure API key for " + p.Name + "?")
			ui.Writeln("")
			if err != nil || !configure {
				ui.Writeln("  " + pterm.FgGray.Sprint("Skipped."))
				ui.Writeln("")
				continue
			}

			apiKey, err := ui.ReadText("API key")
			ui.Writeln("")
			if err != nil {
				if errors.Is(err, ui.ErrCancelled) {
					ui.Writeln("  " + pterm.FgGray.Sprint("Skipped."))
					ui.Writeln("")
					continue
				}
				return err
			}
			if apiKey == "" {
				ui.Writeln("  " + pterm.FgYellow.Sprint("⚠") + "  Empty API key, skipping.")
				ui.Writeln("")
				continue
			}

			ui.Write("  " + pterm.FgGray.Sprint("Saving..."))
			_, err = b.UpdateProvider(p.ID, &models.UpdateModelProviderDto{APIKey: apiKey})
			if err != nil {
				ui.Writeln("\r" + "  " + pterm.FgRed.Sprint("✗") + " Failed: " + pterm.FgRed.Sprint(err.Error()))
				ui.Writeln("")
				continue
			}
			ui.Writeln("\r" + "  " + pterm.FgGreen.Sprint("✓") + " API key saved for " + pterm.FgCyan.Sprint(p.Name) + ".")
			ui.Writeln("")
			configuredIDs[p.ID.String()] = true
		}
	}

	if err := wizardSelectDefaultModel(b, configuredIDs); err != nil && !errors.Is(err, ui.ErrCancelled) {
		return err
	}

	_ = b.SetInitialized()

	ui.Writeln("  " + pterm.FgGreen.Sprint("✓") + " " + pterm.Bold.Sprint("Setup complete!") +
		" " + pterm.FgGray.Sprint("Starting chat..."))
	ui.Writeln("")
	return nil
}

func wizardSelectDefaultModel(b backend.Backend, configuredProviderIDs map[string]bool) error {
	modelList, err := b.ListModels(1, 200)
	if err != nil {
		return err
	}
	if len(modelList.Data) == 0 {
		return nil
	}

	fmt.Printf("\n  %s\n  %s\n\n",
		pterm.Bold.Sprint("Select Default Model"),
		pterm.FgGray.Sprint(strings.Repeat("─", 58)))

	var labels []string
	for _, m := range modelList.Data {
		label := ""
		if m.Provider != nil {
			configured := configuredProviderIDs[m.Provider.ID.String()]
			providerTag := pterm.FgGray.Sprint(m.Provider.Name)
			if configured {
				providerTag = pterm.FgCyan.Sprint(m.Provider.Name)
			}
			label = providerTag + pterm.FgGray.Sprint("/") + m.Name
		} else {
			label = m.Name
		}
		if m.DefaultModel {
			label += " " + pterm.FgGreen.Sprint("(current default)")
		}
		labels = append(labels, label)
	}

	defaultIdx := 0
	for i, m := range modelList.Data {
		if m.DefaultModel {
			defaultIdx = i
			break
		}
	}

	ui.Writeln("  " + pterm.FgGray.Sprintf(
		"Providers with %s have a configured API key.",
		pterm.FgCyan.Sprint("cyan names")))
	ui.Writeln("")

	idx, err := ui.SelectOption("Choose a default model", labels, defaultIdx)
	ui.Writeln("")
	if err != nil {
		return err
	}

	selected := modelList.Data[idx]
	if selected.DefaultModel {
		ui.Writeln("  " + pterm.FgGray.Sprint("Default model unchanged."))
		ui.Writeln("")
		return nil
	}

	isDefault := true
	ui.Write("  " + pterm.FgGray.Sprint("Setting default model..."))
	if err := b.UpdateModel(selected.ID, &models.UpdateModelDto{DefaultModel: &isDefault}); err != nil {
		ui.Writeln("\r" + "  " + pterm.FgRed.Sprint("✗") + " Failed: " + pterm.FgRed.Sprint(err.Error()))
		ui.Writeln("")
		return nil
	}
	ui.Writeln("\r" + "  " + pterm.FgGreen.Sprint("✓") + " Default model set to " +
		pterm.FgMagenta.Sprint(selected.Name) + ".")
	ui.Writeln("")
	return nil
}

