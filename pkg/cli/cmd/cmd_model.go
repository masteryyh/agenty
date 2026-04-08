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

func handleModelCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	if len(args) > 0 {
		resolvedID, displayName, err := resolveModel(b, args[0])
		if err != nil {
			return CommandResult{Handled: true}, err
		}
		pterm.Success.Printf("Switched to model: %s\n", displayName)
		return CommandResult{Handled: true, NewModelID: resolvedID}, nil
	}

	for {
		result, err := b.ListModels(1, 100)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to list models: %w", err)
		}

		if len(result.Data) == 0 {
			pterm.Warning.Println("No models found")
			fmt.Printf("  %s\n", pterm.FgGray.Sprint("Press 'a' to add a model, or Esc to go back"))
			res, err := ui.ShowList("Models", []string{"(no models)"}, "a add  ·  Esc back")
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if res.Action == ui.ListActionAdd {
				if err := doCreateModel(b); err != nil && !errors.Is(err, ui.ErrCancelled) {
					pterm.Error.Printf("Failed to create model: %v\n", err)
				}
				continue
			}
			return CommandResult{Handled: true}, nil
		}

		items := make([]string, len(result.Data))
		for i, m := range result.Data {
			items[i] = modelLabel(m)
		}

		res, err := ui.ShowList("Models  "+pterm.FgGray.Sprint("(select to switch)"), items, listHints)
		if err != nil {
			return CommandResult{Handled: true}, err
		}

		switch res.Action {
		case ui.ListActionSelect:
			target := result.Data[res.Index]
			if target.EmbeddingModel {
				pterm.Warning.Println("Embedding models cannot be used as chat models.")
				continue
			}
			pterm.Success.Printf("Switched to model: %s\n", modelDisplayName(target))
			return CommandResult{Handled: true, NewModelID: target.ID}, nil

		case ui.ListActionAdd:
			if err := doCreateModel(b); err != nil && !errors.Is(err, ui.ErrCancelled) {
				pterm.Error.Printf("Failed to create model: %v\n", err)
			}
			continue

		case ui.ListActionEdit:
			if err := doUpdateModel(b, result.Data[res.Index]); err != nil && !errors.Is(err, ui.ErrCancelled) {
				pterm.Error.Printf("Failed to update model: %v\n", err)
			}
			continue

		case ui.ListActionDelete:
			target := result.Data[res.Index]
			confirmed, err := ui.ShowConfirm(fmt.Sprintf("Delete model '%s/%s'?", target.Provider.Name, target.Name))
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if confirmed {
				if err := b.DeleteModel(target.ID); err != nil {
					pterm.Error.Printf("Failed to delete model: %v\n", err)
				} else {
					pterm.Success.Printf("Model deleted: %s/%s\n", target.Provider.Name, target.Name)
				}
			}
			continue

		case ui.ListActionCancel:
			return CommandResult{Handled: true}, nil
		}
	}
}

func modelLabel(m models.ModelDto) string {
	providerName := ""
	if m.Provider != nil {
		providerName = m.Provider.Name
	}
	var tags []string
	if m.DefaultModel {
		tags = append(tags, pterm.FgGreen.Sprint("[D]"))
	}
	if m.EmbeddingModel {
		tags = append(tags, pterm.FgCyan.Sprint("[E]"))
	}
	if m.ContextCompressionModel {
		tags = append(tags, pterm.FgYellow.Sprint("[CC]"))
	}
	tagStr := ""
	if len(tags) > 0 {
		tagStr = " " + strings.Join(tags, "")
	}
	return fmt.Sprintf("%s/%s (%s)%s", providerName, m.Name, m.Code, tagStr)
}

func doCreateModel(b backend.Backend) error {
	providers, err := b.ListProviders(1, 100)
	if err != nil {
		return fmt.Errorf("failed to list providers: %w", err)
	}
	if len(providers.Data) == 0 {
		pterm.Warning.Println("No providers available. Use /provider to create one first.")
		return nil
	}

	providerOptions := make([]string, len(providers.Data))
	for i, p := range providers.Data {
		providerOptions[i] = providerLabel(p)
	}

	fields := []*ui.FormField{
		ui.SelectField("Provider", providerOptions, 0),
		ui.TextField("Name", "", true),
		ui.TextField("Code", "", true),
		ui.SelectField("Type", []string{"Chat", "Chat + Context compression", "Embedding"}, 0),
	}

	submitted, err := ui.ShowForm("Create Model", fields)
	if err != nil {
		return err
	}
	if !submitted {
		return ui.ErrCancelled
	}

	targetProvider := providers.Data[fields[0].SelIdx]
	name := fields[1].Value
	code := fields[2].Value
	typeIdx := fields[3].SelIdx
	embeddingModel := typeIdx == 2
	contextCompressionModel := typeIdx == 1

	model, err := b.CreateModel(&models.CreateModelDto{
		Name:                    name,
		Code:                    code,
		ProviderID:              targetProvider.ID,
		EmbeddingModel:          embeddingModel,
		ContextCompressionModel: contextCompressionModel,
	})
	if err != nil {
		return err
	}
	pterm.Success.Printf("Model created: %s (%s) under %s\n", model.Name, model.Code, targetProvider.Name)

	if embeddingModel {
		if err := offerSetActiveEmbeddingModel(b, model.ID); err != nil {
			return err
		}
	}

	if contextCompressionModel {
		setActive, err := ui.ShowConfirm("Set as active context compression model in system settings?")
		if err != nil {
			return err
		}
		if setActive {
			if _, err := b.UpdateSystemSettings(&models.UpdateSystemSettingsDto{ContextCompressionModelID: &model.ID}); err != nil {
				pterm.Warning.Printf("Failed to set active context compression model: %v\n", err)
			} else {
				pterm.Success.Println("Active context compression model updated.")
			}
		}
	}

	return nil
}

func doUpdateModel(b backend.Backend, target models.ModelDto) error {
	currentTypeIdx := 0
	if target.ContextCompressionModel {
		currentTypeIdx = 1
	} else if target.EmbeddingModel {
		currentTypeIdx = 2
	}

	fields := []*ui.FormField{
		ui.TextField("Name", target.Name, true),
		ui.ToggleField("Default model", target.DefaultModel),
		ui.SelectField("Type", []string{"Chat", "Chat + Context compression", "Embedding"}, currentTypeIdx),
	}

	submitted, err := ui.ShowForm("Update Model  "+pterm.FgGray.Sprint(target.Code), fields)
	if err != nil {
		return err
	}
	if !submitted {
		return ui.ErrCancelled
	}

	newName := fields[0].Value
	setDefault := fields[1].BoolValue()
	typeIdx := fields[2].SelIdx
	embeddingModel := typeIdx == 2
	contextCompressionModel := typeIdx == 1

	if newName == target.Name && setDefault == target.DefaultModel &&
		embeddingModel == target.EmbeddingModel && contextCompressionModel == target.ContextCompressionModel {
		pterm.Info.Println("No changes detected, skipping update")
		return nil
	}

	if err := b.UpdateModel(target.ID, &models.UpdateModelDto{
		Name:                    &newName,
		DefaultModel:            &setDefault,
		EmbeddingModel:          &embeddingModel,
		ContextCompressionModel: &contextCompressionModel,
	}); err != nil {
		return err
	}
	pterm.Success.Printf("Model updated: %s\n", newName)

	if embeddingModel && !target.EmbeddingModel {
		if err := offerSetActiveEmbeddingModel(b, target.ID); err != nil {
			return err
		}
	}

	if contextCompressionModel && !target.ContextCompressionModel {
		setActive, err := ui.ShowConfirm("Set as active context compression model in system settings?")
		if err != nil {
			return err
		}
		if setActive {
			if _, err := b.UpdateSystemSettings(&models.UpdateSystemSettingsDto{ContextCompressionModelID: &target.ID}); err != nil {
				pterm.Warning.Printf("Failed to set active context compression model: %v\n", err)
			} else {
				pterm.Success.Println("Active context compression model updated.")
			}
		}
	}

	return nil
}

func offerSetActiveEmbeddingModel(b backend.Backend, modelID uuid.UUID) error {
	setActive, err := ui.ShowConfirm("Set as active embedding model in system settings?")
	if err != nil {
		return err
	}
	if !setActive {
		return nil
	}

	settings, err := b.GetSystemSettings()
	if err != nil {
		pterm.Warning.Printf("Failed to get system settings: %v\n", err)
		return nil
	}

	if settings.EmbeddingModelID != nil && *settings.EmbeddingModelID != modelID {
		pterm.Warning.Println("Switching the embedding model will trigger re-generation of ALL existing")
		pterm.Warning.Println("embedding data in the background. This may take time and incur API costs.")
		confirmed, err := ui.ShowConfirm("Proceed with switching the active embedding model?")
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	if _, err := b.UpdateSystemSettings(&models.UpdateSystemSettingsDto{EmbeddingModelID: &modelID}); err != nil {
		pterm.Warning.Printf("Failed to set active embedding model: %v\n", err)
		return nil
	}
	pterm.Success.Println("Active embedding model updated.")
	pterm.Info.Println("Re-embedding in progress in background.")
	return nil
}
