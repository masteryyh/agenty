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

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage agents",
	Long:  `Create, list, update and delete agents`,
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all agents",
	RunE: func(cmd *cobra.Command, args []string) error {
		printSection("Agents")
		c := GetClient()

		page, _ := cmd.Flags().GetInt("page")
		pageSize, _ := cmd.Flags().GetInt("page-size")

		result, err := c.ListAgents(page, pageSize)
		if err != nil {
			return err
		}

		if len(result.Data) == 0 {
			pterm.Warning.Println("No agents found")
			return nil
		}

		tableData := pterm.TableData{
			{"Name", "Default", "ID", "Created"},
		}
		for _, a := range result.Data {
			defaultMark := ""
			if a.IsDefault {
				defaultMark = "✓"
			}
			tableData = append(tableData, []string{a.Name, defaultMark, a.ID.String(), a.CreatedAt.Format("2006-01-02 15:04:05")})
		}
		pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
		fmt.Printf("  %s\n", pterm.FgGray.Sprintf("Total: %d  ·  Page %d/%d", result.Total, result.Page, (result.Total+int64(result.PageSize)-1)/int64(result.PageSize)))
		return nil
	},
}

var agentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("must be run in an interactive terminal")
		}

		printSection("Create Agent")
		c := GetClient()

		name, err := pterm.DefaultInteractiveTextInput.Show("Agent name")
		if err != nil {
			return err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("name cannot be empty")
		}

		soul, err := pterm.DefaultInteractiveTextInput.Show("Agent soul (system prompt), leave empty for default")
		if err != nil {
			return err
		}
		soul = strings.TrimSpace(soul)

		isDefault, err := pterm.DefaultInteractiveConfirm.
			WithDefaultValue(false).
			Show("Set as default agent?")
		if err != nil {
			return err
		}

		agent, err := c.CreateAgent(&models.CreateAgentDto{
			Name:      name,
			Soul:      &soul,
			IsDefault: isDefault,
		})
		if err != nil {
			return err
		}
		pterm.Success.Printf("Agent created: %s (%s)\n", agent.Name, agent.ID)
		return nil
	},
}

var agentUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update an agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("must be run in an interactive terminal")
		}

		printSection("Update Agent")
		c := GetClient()

		result, err := c.ListAgents(1, 100)
		if err != nil {
			return fmt.Errorf("failed to list agents: %w", err)
		}
		if len(result.Data) == 0 {
			pterm.Warning.Println("No agents found")
			return nil
		}

		options := make([]string, len(result.Data))
		for i, a := range result.Data {
			options[i] = a.Name
		}
		selected, err := pterm.DefaultInteractiveSelect.WithOptions(options).Show("Select agent to update")
		if err != nil {
			return err
		}
		var target models.AgentDto
		for _, a := range result.Data {
			if a.Name == selected {
				target = a
				break
			}
		}

		newName, err := pterm.DefaultInteractiveTextInput.WithDefaultText(target.Name).Show("Agent name")
		if err != nil {
			return err
		}
		newName = strings.TrimSpace(newName)
		if newName == "" {
			newName = target.Name
		}

		newSoul, err := pterm.DefaultInteractiveTextInput.WithDefaultText(target.Soul).Show("Agent soul (system prompt)")
		if err != nil {
			return err
		}
		newSoul = strings.TrimSpace(newSoul)
		if newSoul == "" {
			newSoul = target.Soul
		}

		newIsDefault, err := pterm.DefaultInteractiveConfirm.
			WithDefaultValue(target.IsDefault).
			Show("Set as default agent?")
		if err != nil {
			return err
		}

		if newName == target.Name && newSoul == target.Soul && newIsDefault == target.IsDefault {
			pterm.Info.Println("No changes detected, skipping update")
			return nil
		}

		if err := c.UpdateAgent(target.ID, &models.UpdateAgentDto{
			Name:      &newName,
			Soul:      &newSoul,
			IsDefault: &newIsDefault,
		}); err != nil {
			return err
		}
		pterm.Success.Printf("Agent updated: %s\n", newName)
		return nil
	},
}

var agentDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete an agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("must be run in an interactive terminal")
		}

		printSection("Delete Agent")
		c := GetClient()

		result, err := c.ListAgents(1, 100)
		if err != nil {
			return fmt.Errorf("failed to list agents: %w", err)
		}
		if len(result.Data) == 0 {
			pterm.Warning.Println("No agents found")
			return nil
		}

		options := make([]string, len(result.Data))
		for i, a := range result.Data {
			options[i] = a.Name
		}
		selected, err := pterm.DefaultInteractiveSelect.WithOptions(options).Show("Select agent to delete")
		if err != nil {
			return err
		}
		var target models.AgentDto
		for _, a := range result.Data {
			if a.Name == selected {
				target = a
				break
			}
		}

		confirm, err := pterm.DefaultInteractiveConfirm.Show(fmt.Sprintf("Delete agent '%s'? This will also delete all associated sessions, messages and memories.", target.Name))
		if err != nil {
			return err
		}
		if !confirm {
			pterm.Info.Println("Deletion cancelled")
			return nil
		}

		if err := c.DeleteAgent(target.ID); err != nil {
			return err
		}
		pterm.Success.Printf("Agent deleted: %s\n", target.Name)
		return nil
	},
}

func init() {
	agentListCmd.Flags().Int("page", 1, "Page number")
	agentListCmd.Flags().Int("page-size", 20, "Page size")

	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentCreateCmd)
	agentCmd.AddCommand(agentUpdateCmd)
	agentCmd.AddCommand(agentDeleteCmd)

	rootCmd.AddCommand(agentCmd)
}
