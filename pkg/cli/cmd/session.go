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
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage chat sessions",
	Long:  `Create, list and view chat sessions`,
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.NewClient(GetBaseURL())
		
		page, _ := cmd.Flags().GetInt("page")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		
		result, err := c.ListSessions(page, pageSize)
		if err != nil {
			return err
		}
		
		if len(result.Data) == 0 {
			pterm.Warning.Println("No sessions found")
			return nil
		}
		
		tableData := pterm.TableData{
			{"ID", "Token Consumed", "Created At", "Updated At"},
		}
		
		for _, s := range result.Data {
			tableData = append(tableData, []string{
				s.ID.String(),
				fmt.Sprintf("%d", s.TokenConsumed),
				s.CreatedAt.Format("2006-01-02 15:04:05"),
				s.UpdatedAt.Format("2006-01-02 15:04:05"),
			})
		}
		
		pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
		pterm.Info.Printf("Total: %d, Page: %d/%d\n", result.Total, result.Page, (result.Total+int64(result.PageSize)-1)/int64(result.PageSize))
		
		return nil
	},
}

var sessionCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new session",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.NewClient(GetBaseURL())
		
		session, err := c.CreateSession()
		if err != nil {
			return err
		}
		
		pterm.Success.Printf("Session created: %s\n", session.ID)
		return nil
	},
}

var sessionViewCmd = &cobra.Command{
	Use:   "view [session-id]",
	Short: "View session details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.NewClient(GetBaseURL())
		
		sessionID, err := uuid.Parse(args[0])
		if err != nil {
			return fmt.Errorf("invalid session ID: %w", err)
		}
		
		session, err := c.GetSession(sessionID)
		if err != nil {
			return err
		}
		
		pterm.DefaultHeader.Printf("Session: %s\n", session.ID)
		pterm.Info.Printf("Token Consumed: %d\n", session.TokenConsumed)
		pterm.Info.Printf("Created At: %s\n", session.CreatedAt.Format("2006-01-02 15:04:05"))
		pterm.Info.Printf("Updated At: %s\n", session.UpdatedAt.Format("2006-01-02 15:04:05"))
		fmt.Println()
		
		if len(session.Messages) == 0 {
			pterm.Warning.Println("No messages in this session")
			return nil
		}
		
		pterm.DefaultSection.Println("Messages")
		for _, msg := range session.Messages {
			printMessage(&msg)
		}
		
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sessionCmd)
	
	sessionCmd.AddCommand(sessionListCmd)
	sessionListCmd.Flags().Int("page", 1, "Page number")
	sessionListCmd.Flags().Int("page-size", 10, "Page size")
	
	sessionCmd.AddCommand(sessionCreateCmd)
	sessionCmd.AddCommand(sessionViewCmd)
}
