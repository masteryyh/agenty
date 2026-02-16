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

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/cli/client"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
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
		c := client.NewClient(GetBaseURL())

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
			{"ID", "Name", "Type", "Base URL", "API Key"},
		}

		for _, p := range result.Data {
			tableData = append(tableData, []string{
				p.ID.String(),
				p.Name,
				string(p.Type),
				p.BaseURL,
				p.APIKeyCensored,
			})
		}

		pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
		pterm.Info.Printf("Total: %d, Page: %d/%d\n", result.Total, result.Page, (result.Total+int64(result.PageSize)-1)/int64(result.PageSize))

		return nil
	},
}

var providerCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.NewClient(GetBaseURL())

		name, _ := cmd.Flags().GetString("name")
		apiType, _ := cmd.Flags().GetString("type")
		baseURLFlag, _ := cmd.Flags().GetString("base-url")
		apiKey, _ := cmd.Flags().GetString("api-key")

		dto := &models.CreateModelProviderDto{
			Name:    name,
			Type:    models.APIType(apiType),
			BaseURL: baseURLFlag,
			APIKey:  apiKey,
		}

		provider, err := c.CreateProvider(dto)
		if err != nil {
			return err
		}

		pterm.Success.Printf("Provider created: %s (ID: %s)\n", provider.Name, provider.ID)
		return nil
	},
}

var providerDeleteCmd = &cobra.Command{
	Use:   "delete [provider-id]",
	Short: "Delete a provider",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.NewClient(GetBaseURL())

		providerID, err := uuid.Parse(args[0])
		if err != nil {
			return fmt.Errorf("invalid provider ID: %w", err)
		}

		force, _ := cmd.Flags().GetBool("force")

		if err := c.DeleteProvider(providerID, force); err != nil {
			return err
		}

		pterm.Success.Printf("Provider deleted: %s\n", providerID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(providerCmd)

	providerCmd.AddCommand(providerListCmd)
	providerListCmd.Flags().Int("page", 1, "Page number")
	providerListCmd.Flags().Int("page-size", 10, "Page size")

	providerCmd.AddCommand(providerCreateCmd)
	providerCreateCmd.Flags().String("name", "", "Provider name (required)")
	providerCreateCmd.Flags().String("type", "", "Provider type: openai, anthropic, kimi, gemini (required)")
	providerCreateCmd.Flags().String("base-url", "", "Base URL (required)")
	providerCreateCmd.Flags().String("api-key", "", "API key (required)")
	providerCreateCmd.MarkFlagRequired("name")
	providerCreateCmd.MarkFlagRequired("type")
	providerCreateCmd.MarkFlagRequired("base-url")
	providerCreateCmd.MarkFlagRequired("api-key")

	providerCmd.AddCommand(providerDeleteCmd)
	providerDeleteCmd.Flags().Bool("force", false, "Force delete (also delete associated models)")
}
