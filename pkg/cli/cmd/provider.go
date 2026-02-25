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

var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage model providers",
	Long:  `Create, list, update and delete model providers`,
}

var providerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all providers",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()

		page, _ := cmd.Flags().GetInt("page")
		pageSize, _ := cmd.Flags().GetInt("page-size")

		result, err := c.ListProviders(page, pageSize)
		if err != nil {
			return err
		}

		if len(result.Data) == 0 {
			pterm.Warning.Println("No providers found")
			return nil
		}

		tableData := pterm.TableData{
			{"Name", "Type", "Base URL", "API Key"},
		}
		for _, p := range result.Data {
			tableData = append(tableData, []string{p.Name, string(p.Type), p.BaseURL, p.APIKeyCensored})
		}
		pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
		pterm.Info.Printf("Total: %d, Page: %d/%d\n", result.Total, result.Page, (result.Total+int64(result.PageSize)-1)/int64(result.PageSize))
		return nil
	},
}

var providerTypeOptions = []string{"openai", "anthropic", "gemini", "kimi"}

var providerDefaultBaseURLs = map[string]string{
	"openai":    "https://api.openai.com/v1",
	"anthropic": "https://api.anthropic.com",
	"gemini":    "https://generativelanguage.googleapis.com",
	"kimi":      "https://api.moonshot.cn/v1",
}

func readHiddenInput(prompt string) (string, error) {
	pterm.DefaultInteractiveTextInput.TextStyle.Print(prompt + ": ")
	key, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(key)), nil
}

func providerLabel(p models.ModelProviderDto) string {
	return fmt.Sprintf("%s (%s)", p.Name, string(p.Type))
}

var providerCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("must be run in an interactive terminal")
		}

		c := GetClient()

		name, err := pterm.DefaultInteractiveTextInput.Show("Provider name")
		if err != nil {
			return err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("name cannot be empty")
		}

		selectedType, err := pterm.DefaultInteractiveSelect.
			WithOptions(providerTypeOptions).
			Show("Provider type")
		if err != nil {
			return err
		}

		baseURL, err := pterm.DefaultInteractiveTextInput.
			WithDefaultValue(providerDefaultBaseURLs[selectedType]).
			Show("Base URL")
		if err != nil {
			return err
		}
		baseURL = strings.TrimSpace(baseURL)
		if baseURL == "" {
			return fmt.Errorf("base URL cannot be empty")
		}

		apiKey, err := readHiddenInput("API key")
		if err != nil {
			return err
		}

		provider, err := c.CreateProvider(&models.CreateModelProviderDto{
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
	},
}

var providerUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update a provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("must be run in an interactive terminal")
		}

		c := GetClient()

		result, err := c.ListProviders(1, 100)
		if err != nil {
			return fmt.Errorf("failed to list providers: %w", err)
		}
		if len(result.Data) == 0 {
			pterm.Warning.Println("No providers found")
			return nil
		}

		options := make([]string, len(result.Data))
		for i, p := range result.Data {
			options[i] = providerLabel(p)
		}
		selected, err := pterm.DefaultInteractiveSelect.WithOptions(options).Show("Select provider to update")
		if err != nil {
			return err
		}
		var target models.ModelProviderDto
		for _, p := range result.Data {
			if providerLabel(p) == selected {
				target = p
				break
			}
		}

		pterm.Info.Println("Press Enter to keep the current value")

		newName, err := pterm.DefaultInteractiveTextInput.WithDefaultValue(target.Name).Show("Name")
		if err != nil {
			return err
		}
		newName = strings.TrimSpace(newName)
		if newName == "" {
			newName = target.Name
		}

		newType, err := pterm.DefaultInteractiveSelect.
			WithOptions(providerTypeOptions).
			WithDefaultOption(string(target.Type)).
			Show("Type")
		if err != nil {
			return err
		}

		newBaseURL, err := pterm.DefaultInteractiveTextInput.WithDefaultValue(target.BaseURL).Show("Base URL")
		if err != nil {
			return err
		}
		newBaseURL = strings.TrimSpace(newBaseURL)
		if newBaseURL == "" {
			newBaseURL = target.BaseURL
		}

		newAPIKey, err := readHiddenInput("API key (leave empty to keep unchanged)")
		if err != nil {
			return err
		}
		newAPIKey = strings.TrimSpace(newAPIKey)

		if target.Name == newName && string(target.Type) == newType && target.BaseURL == newBaseURL && newAPIKey == "" {
			pterm.Info.Println("No changes detected, skipping update")
			return nil
		}

		updated, err := c.UpdateProvider(target.ID, &models.UpdateModelProviderDto{
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
	},
}

var providerDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("must be run in an interactive terminal")
		}

		c := GetClient()

		result, err := c.ListProviders(1, 100)
		if err != nil {
			return fmt.Errorf("failed to list providers: %w", err)
		}
		if len(result.Data) == 0 {
			pterm.Warning.Println("No providers found")
			return nil
		}

		options := make([]string, len(result.Data))
		for i, p := range result.Data {
			options[i] = providerLabel(p)
		}
		selected, err := pterm.DefaultInteractiveSelect.WithOptions(options).Show("Select provider to delete")
		if err != nil {
			return err
		}
		var target models.ModelProviderDto
		for _, p := range result.Data {
			if providerLabel(p) == selected {
				target = p
				break
			}
		}

		force, err := pterm.DefaultInteractiveConfirm.Show(fmt.Sprintf("Also delete all models under '%s'?", target.Name))
		if err != nil {
			return err
		}

		confirm, err := pterm.DefaultInteractiveConfirm.Show(fmt.Sprintf("Delete provider '%s'?", target.Name))
		if err != nil {
			return err
		}
		if !confirm {
			pterm.Info.Println("Cancelled")
			return nil
		}

		if err := c.DeleteProvider(target.ID, force); err != nil {
			return err
		}
		pterm.Success.Printf("Provider deleted: %s\n", target.Name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(providerCmd)

	providerCmd.AddCommand(providerListCmd)
	providerListCmd.Flags().Int("page", 1, "Page number")
	providerListCmd.Flags().Int("page-size", 10, "Page size")

	providerCmd.AddCommand(providerCreateCmd)
	providerCmd.AddCommand(providerUpdateCmd)
	providerCmd.AddCommand(providerDeleteCmd)
}
