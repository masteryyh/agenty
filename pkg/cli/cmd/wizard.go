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

	rawWriteln("")
	rawWriteln("  " + pterm.Bold.Sprint("First-Time Setup"))
	rawWriteln("  " + divider)
	rawWriteln("")
	rawWriteln("  " + pterm.FgGray.Sprint("Agenty comes with preset LLM providers and models."))
	rawWriteln("  " + pterm.FgGray.Sprint("Let's configure the API keys for the ones you want to use,"))
	rawWriteln("  " + pterm.FgGray.Sprint("and choose your default model."))
	rawWriteln("")
	rawWriteln("  " + pterm.FgGray.Sprintf("Press %s at any time to skip to the next step.",
		pterm.FgWhite.Sprint("Esc")))
	rawWriteln("")

	proceed, err := showConfirm("Start setup now?")
	rawWriteln("")
	if err != nil || !proceed {
		rawWriteln("  " + pterm.FgYellow.Sprint("⚠") + "  Skipping setup. The wizard will appear again on next startup.")
		rawWriteln("     Use " + pterm.FgWhite.Sprint("/provider") + " to configure providers manually.")
		rawWriteln("")
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
			rawWriteln(fmt.Sprintf("  %s  %s",
				pterm.FgCyan.Sprintf("[%d/%d]", i+1, len(providers.Data)),
				pterm.Bold.Sprint(p.Name)+" "+pterm.FgGray.Sprint("("+string(p.Type)+")"),
			))
			rawWriteln("  " + pterm.FgGray.Sprint("Base URL: "+p.BaseURL))
			rawWriteln("")

			configure, err := showConfirm("Configure API key for " + p.Name + "?")
			rawWriteln("")
			if err != nil || !configure {
				rawWriteln("  " + pterm.FgGray.Sprint("Skipped."))
				rawWriteln("")
				continue
			}

			apiKey, err := readText("API key")
			rawWriteln("")
			if err != nil {
				if errors.Is(err, ErrCancelled) {
					rawWriteln("  " + pterm.FgGray.Sprint("Skipped."))
					rawWriteln("")
					continue
				}
				return err
			}
			if apiKey == "" {
				rawWriteln("  " + pterm.FgYellow.Sprint("⚠") + "  Empty API key, skipping.")
				rawWriteln("")
				continue
			}

			rawWrite("  " + pterm.FgGray.Sprint("Saving..."))
			_, err = b.UpdateProvider(p.ID, &models.UpdateModelProviderDto{APIKey: apiKey})
			if err != nil {
				rawWriteln("\r" + "  " + pterm.FgRed.Sprint("✗") + " Failed: " + pterm.FgRed.Sprint(err.Error()))
				rawWriteln("")
				continue
			}
			rawWriteln("\r" + "  " + pterm.FgGreen.Sprint("✓") + " API key saved for " + pterm.FgCyan.Sprint(p.Name) + ".")
			rawWriteln("")
			configuredIDs[p.ID.String()] = true
		}
	}

	if err := wizardSelectDefaultModel(b, configuredIDs); err != nil && !errors.Is(err, ErrCancelled) {
		return err
	}

	_ = b.SetInitialized()

	rawWriteln("  " + pterm.FgGreen.Sprint("✓") + " " + pterm.Bold.Sprint("Setup complete!") +
		" " + pterm.FgGray.Sprint("Starting chat..."))
	rawWriteln("")
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

	rawWriteln("  " + pterm.FgGray.Sprintf(
		"Providers with %s have a configured API key.",
		pterm.FgCyan.Sprint("cyan names")))
	rawWriteln("")

	idx, err := selectOption("Choose a default model", labels, defaultIdx)
	rawWriteln("")
	if err != nil {
		return err
	}

	selected := modelList.Data[idx]
	if selected.DefaultModel {
		rawWriteln("  " + pterm.FgGray.Sprint("Default model unchanged."))
		rawWriteln("")
		return nil
	}

	isDefault := true
	rawWrite("  " + pterm.FgGray.Sprint("Setting default model..."))
	if err := b.UpdateModel(selected.ID, &models.UpdateModelDto{DefaultModel: &isDefault}); err != nil {
		rawWriteln("\r" + "  " + pterm.FgRed.Sprint("✗") + " Failed: " + pterm.FgRed.Sprint(err.Error()))
		rawWriteln("")
		return nil
	}
	rawWriteln("\r" + "  " + pterm.FgGreen.Sprint("✓") + " Default model set to " +
		pterm.FgMagenta.Sprint(selected.Name) + ".")
	rawWriteln("")
	return nil
}

