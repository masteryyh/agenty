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
	"fmt"
	"os"
	"strings"

	"github.com/masteryyh/agenty/pkg/models"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Manage models",
	Long:  `Create, list, update and delete models`,
}

var modelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all models",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()

		page, _ := cmd.Flags().GetInt("page")
		pageSize, _ := cmd.Flags().GetInt("page-size")

		result, err := c.ListModels(page, pageSize)
		if err != nil {
			return err
		}

		if len(result.Data) == 0 {
			pterm.Warning.Println("No models found")
			return nil
		}

		tableData := pterm.TableData{
			{"Name", "Code", "Provider", "Default"},
		}
		for _, m := range result.Data {
			providerName := ""
			if m.Provider != nil {
				providerName = m.Provider.Name
			}
			isDefault := ""
			if m.DefaultModel {
				isDefault = "✓"
			}
			tableData = append(tableData, []string{m.Name, m.Code, providerName, isDefault})
		}
		pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
		pterm.Info.Printf("Total: %d, Page: %d/%d\n", result.Total, result.Page, (result.Total+int64(result.PageSize)-1)/int64(result.PageSize))
		return nil
	},
}

func modelLabel(m models.ModelDto) string {
	providerName := ""
	if m.Provider != nil {
		providerName = m.Provider.Name
	}
	marker := ""
	if m.DefaultModel {
		marker = " [default]"
	}
	return fmt.Sprintf("%s/%s (%s)%s", providerName, m.Name, m.Code, marker)
}

var modelCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new model",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("must be run in an interactive terminal")
		}

		c := GetClient()

		providers, err := c.ListProviders(1, 100)
		if err != nil {
			return fmt.Errorf("failed to list providers: %w", err)
		}
		if len(providers.Data) == 0 {
			pterm.Warning.Println("No providers available. Create a provider first.")
			return nil
		}

		providerOptions := make([]string, len(providers.Data))
		for i, p := range providers.Data {
			providerOptions[i] = providerLabel(p)
		}
		selectedProvider, err := pterm.DefaultInteractiveSelect.WithOptions(providerOptions).Show("Select provider")
		if err != nil {
			return err
		}
		var targetProvider models.ModelProviderDto
		for _, p := range providers.Data {
			if providerLabel(p) == selectedProvider {
				targetProvider = p
				break
			}
		}

		name, err := pterm.DefaultInteractiveTextInput.Show("Model display name")
		if err != nil {
			return err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("name cannot be empty")
		}

		code, err := pterm.DefaultInteractiveTextInput.Show("Model API code/ID")
		if err != nil {
			return err
		}
		code = strings.TrimSpace(code)
		if code == "" {
			return fmt.Errorf("code cannot be empty")
		}

		model, err := c.CreateModel(&models.CreateModelDto{
			Name:       name,
			Code:       code,
			ProviderID: targetProvider.ID,
		})
		if err != nil {
			return err
		}
		pterm.Success.Printf("Model created: %s (%s) under %s\n", model.Name, model.Code, targetProvider.Name)
		return nil
	},
}

var modelUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update a model",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("must be run in an interactive terminal")
		}

		c := GetClient()

		result, err := c.ListModels(1, 100)
		if err != nil {
			return fmt.Errorf("failed to list models: %w", err)
		}
		if len(result.Data) == 0 {
			pterm.Warning.Println("No models found")
			return nil
		}

		options := make([]string, len(result.Data))
		for i, m := range result.Data {
			options[i] = modelLabel(m)
		}
		selected, err := pterm.DefaultInteractiveSelect.WithOptions(options).Show("Select model to update")
		if err != nil {
			return err
		}
		var target models.ModelDto
		for i, m := range result.Data {
			if options[i] == selected {
				target = m
				break
			}
		}

		newName, err := pterm.DefaultInteractiveTextInput.WithDefaultText(target.Name).Show("Model display name")
		if err != nil {
			return err
		}
		newName = strings.TrimSpace(newName)

		if newName == "" {
			newName = target.Name
		}

		setDefault, err := pterm.DefaultInteractiveConfirm.Show("Set as default model?")
		if err != nil {
			return err
		}

		if target.Name == newName && target.DefaultModel == setDefault {
			pterm.Info.Println("No changes detected, skipping update")
			return nil
		}

		if err := c.UpdateModel(target.ID, &models.UpdateModelDto{Name: newName, DefaultModel: setDefault}); err != nil {
			return err
		}
		pterm.Success.Printf("Model updated: %s\n", newName)
		return nil
	},
}

var modelDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a model",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("must be run in an interactive terminal")
		}

		c := GetClient()

		result, err := c.ListModels(1, 100)
		if err != nil {
			return fmt.Errorf("failed to list models: %w", err)
		}
		if len(result.Data) == 0 {
			pterm.Warning.Println("No models found")
			return nil
		}

		options := make([]string, len(result.Data))
		for i, m := range result.Data {
			options[i] = modelLabel(m)
		}
		selected, err := pterm.DefaultInteractiveSelect.WithOptions(options).Show("Select model to delete")
		if err != nil {
			return err
		}
		var target models.ModelDto
		for i, m := range result.Data {
			if options[i] == selected {
				target = m
				break
			}
		}

		providerName := ""
		if target.Provider != nil {
			providerName = target.Provider.Name
		}
		modelRef := fmt.Sprintf("%s/%s", providerName, target.Name)

		confirm, err := pterm.DefaultInteractiveConfirm.Show(fmt.Sprintf("Delete model '%s'?", modelRef))
		if err != nil {
			return err
		}
		if !confirm {
			pterm.Info.Println("Cancelled")
			return nil
		}

		if err := c.DeleteModel(target.ID); err != nil {
			return err
		}
		pterm.Success.Printf("Model deleted: %s\n", modelRef)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(modelCmd)

	modelCmd.AddCommand(modelListCmd)
	modelListCmd.Flags().Int("page", 1, "Page number")
	modelListCmd.Flags().Int("page-size", 10, "Page size")

	modelCmd.AddCommand(modelCreateCmd)
	modelCmd.AddCommand(modelUpdateCmd)
	modelCmd.AddCommand(modelDeleteCmd)
}
