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

func handleModelCmd(b backend.Backend, bridge *UIBridge, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	if len(args) > 0 {
		resolvedID, displayName, err := resolveModel(b, args[0])
		if err != nil {
			return CommandResult{Handled: true}, err
		}
		bridge.Success("Switched to model: %s", displayName)
		return CommandResult{Handled: true, NewModelID: resolvedID, NewModelName: displayName}, nil
	}

	for {
		result, err := b.ListModels(1, 100)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to list models: %w", err)
		}

		var filtered []models.ModelDto
		for _, mdl := range result.Data {
			if mdl.Provider != nil && mdl.Provider.APIKeyCensored != "<not set>" && !mdl.EmbeddingModel {
				filtered = append(filtered, mdl)
			}
		}
		result.Data = filtered

		if len(result.Data) == 0 {
			bridge.Warning("No models from configured providers found")
			res, err := bridge.ShowList("Models", []string{"(no models)"}, "a add  ·  Esc back")
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if res.Action == ListActionAdd {
				if err := doCreateModel(b, bridge); err != nil && !errors.Is(err, ErrCancelled) {
					bridge.Error("Failed to create model: %v", err)
				}
				continue
			}
			return CommandResult{Handled: true}, nil
		}

		items := make([]string, len(result.Data))
		for i, m := range result.Data {
			items[i] = modelLabel(m)
		}

		legend := styleGreen.Render("●") + styleGray.Render(" default  ") +
			styleMagenta.Render("●") + styleGray.Render(" preset  ") +
			styleCyan.Render("●") + styleGray.Render(" light")
		res, err := bridge.ShowList("Models  "+styleGray.Render("(select to switch)"), items, listHints, legend)
		if err != nil {
			return CommandResult{Handled: true}, err
		}

		switch res.Action {
		case ListActionSelect:
			target := result.Data[res.Index]
			displayName := modelDisplayName(target)
			bridge.Success("Switched to model: %s", displayName)
			return CommandResult{Handled: true, NewModelID: target.ID, NewModelName: displayName}, nil

		case ListActionAdd:
			if err := doCreateModel(b, bridge); err != nil && !errors.Is(err, ErrCancelled) {
				bridge.Error("Failed to create model: %v", err)
			}
			continue

		case ListActionEdit:
			if err := doUpdateModel(b, bridge, result.Data[res.Index]); err != nil && !errors.Is(err, ErrCancelled) {
				bridge.Error("Failed to update model: %v", err)
			}
			continue

		case ListActionDelete:
			target := result.Data[res.Index]
			if target.IsPreset {
				bridge.Warning("'%s/%s' is a preset model and cannot be deleted.", target.Provider.Name, target.Name)
				continue
			}
			confirmed, err := bridge.ShowConfirm(fmt.Sprintf("Delete model '%s/%s'?", target.Provider.Name, target.Name))
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if confirmed {
				if err := b.DeleteModel(target.ID); err != nil {
					bridge.Error("Failed to delete model: %v", err)
				} else {
					bridge.Success("Model deleted: %s/%s", target.Provider.Name, target.Name)
				}
			}
			continue

		case ListActionCancel:
			return CommandResult{Handled: true}, nil
		}
	}
}

func modelLabel(m models.ModelDto) string {
	providerName := ""
	if m.Provider != nil {
		providerName = m.Provider.Name
	}
	var dots []string
	if m.DefaultModel {
		dots = append(dots, styleGreen.Render("●"))
	}
	if m.IsPreset {
		dots = append(dots, styleMagenta.Render("●"))
	}
	if m.Light {
		dots = append(dots, styleCyan.Render("●"))
	}
	dotStr := ""
	if len(dots) > 0 {
		dotStr = " " + strings.Join(dots, "")
	}
	return fmt.Sprintf("%s/%s (%s)%s", providerName, m.Name, m.Code, dotStr)
}

func doCreateModel(b backend.Backend, bridge *UIBridge) error {
	providers, err := b.ListProviders(1, 100)
	if err != nil {
		return fmt.Errorf("failed to list providers: %w", err)
	}
	if len(providers.Data) == 0 {
		bridge.Warning("No providers available. Use /provider to create one first.")
		return nil
	}

	providerOptions := make([]string, len(providers.Data))
	for i, p := range providers.Data {
		providerOptions[i] = providerLabel(p)
	}

	selectedProvider := providerOptions[0]
	var name, code string
	modelType := "Chat"
	lightModel := false

	providerOpts := make([]huh.Option[string], len(providerOptions))
	for i, p := range providerOptions {
		providerOpts[i] = huh.NewOption(p, p)
	}

	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Provider").Options(providerOpts...).Value(&selectedProvider),
		huh.NewInput().Title("Name").Value(&name).Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("required")
			}
			return nil
		}),
		huh.NewInput().Title("Code").Value(&code).Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("required")
			}
			return nil
		}),
		huh.NewSelect[string]().Title("Type").
			Options(
				huh.NewOption("Chat", "Chat"),
				huh.NewOption("Embedding", "Embedding"),
			).Value(&modelType),
		huh.NewSelect[bool]().Title("Lightweight model").
			Options(huh.NewOption("Yes", true), huh.NewOption("No", false)).
			Value(&lightModel),
	))

	submitted, err := bridge.ShowHuhForm(form)
	if err != nil {
		return err
	}
	if !submitted {
		return ErrCancelled
	}

	var targetProvider models.ModelProviderDto
	for i, opt := range providerOptions {
		if opt == selectedProvider {
			targetProvider = providers.Data[i]
			break
		}
	}

	embeddingModel := modelType == "Embedding"

	model, err := b.CreateModel(&models.CreateModelDto{
		Name:           name,
		Code:           code,
		ProviderID:     targetProvider.ID,
		EmbeddingModel: embeddingModel,
		Light:          lightModel,
	})
	if err != nil {
		return err
	}
	bridge.Success("Model created: %s (%s) under %s", model.Name, model.Code, targetProvider.Name)

	if embeddingModel {
		if err := offerSetActiveEmbeddingModel(b, bridge, model.ID); err != nil {
			return err
		}
	}

	return nil
}

func doUpdateModel(b backend.Backend, bridge *UIBridge, target models.ModelDto) error {
	currentTypeIdx := 0
	if target.EmbeddingModel {
		currentTypeIdx = 1
	}

	typeOptions := []string{"Chat", "Embedding"}
	newName := target.Name
	setDefault := target.DefaultModel
	lightModel := target.Light
	modelType := typeOptions[currentTypeIdx]

	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Name").Value(&newName).Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("required")
			}
			return nil
		}),
		huh.NewSelect[bool]().Title("Default model").
			Options(huh.NewOption("Yes", true), huh.NewOption("No", false)).
			Value(&setDefault),
		huh.NewSelect[string]().Title("Type").
			Options(
				huh.NewOption("Chat", "Chat"),
				huh.NewOption("Embedding", "Embedding"),
			).Value(&modelType),
		huh.NewSelect[bool]().Title("Lightweight model").
			Options(huh.NewOption("Yes", true), huh.NewOption("No", false)).
			Value(&lightModel),
	))

	submitted, err := bridge.ShowHuhForm(form)
	if err != nil {
		return err
	}
	if !submitted {
		return ErrCancelled
	}

	newName = strings.TrimSpace(newName)
	embeddingModel := modelType == "Embedding"

	if newName == target.Name && setDefault == target.DefaultModel &&
		embeddingModel == target.EmbeddingModel && lightModel == target.Light {
		bridge.Info("No changes detected, skipping update")
		return nil
	}

	if err := b.UpdateModel(target.ID, &models.UpdateModelDto{
		Name:           &newName,
		DefaultModel:   &setDefault,
		EmbeddingModel: &embeddingModel,
		Light:          &lightModel,
	}); err != nil {
		return err
	}
	bridge.Success("Model updated: %s", newName)

	if embeddingModel && !target.EmbeddingModel {
		if err := offerSetActiveEmbeddingModel(b, bridge, target.ID); err != nil {
			return err
		}
	}

	return nil
}

func offerSetActiveEmbeddingModel(b backend.Backend, bridge *UIBridge, modelID uuid.UUID) error {
	setActive, err := bridge.ShowConfirm("Set as active embedding model in system settings?")
	if err != nil {
		return err
	}
	if !setActive {
		return nil
	}

	settings, err := b.GetSystemSettings()
	if err != nil {
		bridge.Warning("Failed to get system settings: %v", err)
		return nil
	}

	if settings.EmbeddingModelID != nil && *settings.EmbeddingModelID != modelID {
		bridge.Warning("Switching the embedding model will trigger re-generation of ALL existing embedding data in the background. This may take time and incur API costs.")
		confirmed, err := bridge.ShowConfirm("Proceed with switching the active embedding model?")
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	if _, err := b.UpdateSystemSettings(&models.UpdateSystemSettingsDto{EmbeddingModelID: &modelID}); err != nil {
		bridge.Warning("Failed to set active embedding model: %v", err)
		return nil
	}
	bridge.Success("Active embedding model updated.")
	bridge.Info("Re-embedding in progress in background.")
	return nil
}
