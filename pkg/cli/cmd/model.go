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

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Manage models",
	Long:  `Create, list and delete models`,
}

var modelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all models",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.NewClient(GetBaseURL())
		
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
			{"ID", "Name", "Provider"},
		}
		
		for _, m := range result.Data {
			providerName := ""
			if m.Provider != nil {
				providerName = m.Provider.Name
			}
			tableData = append(tableData, []string{
				m.ID.String(),
				m.Name,
				providerName,
			})
		}
		
		pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
		pterm.Info.Printf("Total: %d, Page: %d/%d\n", result.Total, result.Page, (result.Total+int64(result.PageSize)-1)/int64(result.PageSize))
		
		return nil
	},
}

var modelCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new model",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.NewClient(GetBaseURL())
		
		name, _ := cmd.Flags().GetString("name")
		providerIDStr, _ := cmd.Flags().GetString("provider-id")
		
		providerID, err := uuid.Parse(providerIDStr)
		if err != nil {
			return fmt.Errorf("invalid provider ID: %w", err)
		}
		
		dto := &models.CreateModelDto{
			Name:       name,
			ProviderID: providerID,
		}
		
		model, err := c.CreateModel(dto)
		if err != nil {
			return err
		}
		
		pterm.Success.Printf("Model created: %s (ID: %s)\n", model.Name, model.ID)
		return nil
	},
}

var modelDeleteCmd = &cobra.Command{
	Use:   "delete [model-id]",
	Short: "Delete a model",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.NewClient(GetBaseURL())
		
		modelID, err := uuid.Parse(args[0])
		if err != nil {
			return fmt.Errorf("invalid model ID: %w", err)
		}
		
		if err := c.DeleteModel(modelID); err != nil {
			return err
		}
		
		pterm.Success.Printf("Model deleted: %s\n", modelID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(modelCmd)
	
	modelCmd.AddCommand(modelListCmd)
	modelListCmd.Flags().Int("page", 1, "Page number")
	modelListCmd.Flags().Int("page-size", 10, "Page size")
	
	modelCmd.AddCommand(modelCreateCmd)
	modelCreateCmd.Flags().String("name", "", "Model name (required)")
	modelCreateCmd.Flags().String("provider-id", "", "Provider ID (required)")
	modelCreateCmd.MarkFlagRequired("name")
	modelCreateCmd.MarkFlagRequired("provider-id")
	
	modelCmd.AddCommand(modelDeleteCmd)
}
