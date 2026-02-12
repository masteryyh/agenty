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
	"bufio"
	"fmt"
	"os"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/cli/client"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive chat session",
	Long:  `Start an interactive chat session with an AI model`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.NewClient(GetBaseURL())
		
		sessionIDStr, _ := cmd.Flags().GetString("session")
		modelIDStr, _ := cmd.Flags().GetString("model")
		
		if modelIDStr == "" {
			return fmt.Errorf("model ID is required (use --model flag)")
		}
		
		modelID, err := uuid.Parse(modelIDStr)
		if err != nil {
			return fmt.Errorf("invalid model ID: %w", err)
		}
		
		var sessionID uuid.UUID
		var session *models.ChatSessionDto
		
		if sessionIDStr != "" {
			sessionID, err = uuid.Parse(sessionIDStr)
			if err != nil {
				return fmt.Errorf("invalid session ID: %w", err)
			}
			session, err = c.GetSession(sessionID)
			if err != nil {
				return fmt.Errorf("failed to get session: %w", err)
			}
		} else {
			session, err = c.CreateSession()
			if err != nil {
				return fmt.Errorf("failed to create session: %w", err)
			}
			sessionID = session.ID
		}
		
		pterm.Success.Printf("Chat session: %s\n", sessionID)
		fmt.Println()
		
		if len(session.Messages) > 0 {
			pterm.DefaultSection.Println("Previous Messages")
			for _, msg := range session.Messages {
				printMessage(&msg)
			}
			fmt.Println()
		}
		
		pterm.Info.Println("Type your message and press Enter. Type 'exit' to quit.")
		fmt.Println()
		
		scanner := bufio.NewScanner(os.Stdin)
		
		for {
			pterm.Print(pterm.FgCyan.Sprint("You: "))
			if !scanner.Scan() {
				break
			}
			
			input := strings.TrimSpace(scanner.Text())
			if input == "" {
				continue
			}
			
			if strings.ToLower(input) == "exit" {
				pterm.Info.Println("Goodbye!")
				break
			}
			
			spinner, _ := pterm.DefaultSpinner.Start("Thinking...")
			
			messages, err := c.Chat(sessionID, &models.ChatDto{
				ModelID: modelID,
				Message: input,
			})
			
			spinner.Stop()
			
			if err != nil {
				pterm.Error.Printf("Error: %v\n", err)
				continue
			}
			
			fmt.Println()
			for _, msg := range messages {
				printMessage(msg)
			}
			fmt.Println()
		}
		
		return nil
	},
}

func printMessage(msg *models.ChatMessageDto) {
	switch msg.Role {
	case models.RoleUser:
		pterm.Println(pterm.FgCyan.Sprintf("👤 User [%s]:", msg.CreatedAt.Format("15:04:05")))
		pterm.Println(pterm.NewStyle(pterm.FgWhite).Sprint("  " + msg.Content))
		
	case models.RoleAssistant:
		modelInfo := ""
		if msg.Model != nil {
			modelInfo = fmt.Sprintf(" (%s)", msg.Model.Name)
		}
		pterm.Println(pterm.FgGreen.Sprintf("🤖 Assistant%s [%s]:", modelInfo, msg.CreatedAt.Format("15:04:05")))
		
		if msg.ProviderSpecifics != nil && msg.ProviderSpecifics.KimiReasoningContent != "" {
			pterm.Println(pterm.FgBlue.Sprint("  💭 Reasoning:"))
			pterm.Println(pterm.NewStyle(pterm.FgGray).Sprint("  " + msg.ProviderSpecifics.KimiReasoningContent))
			fmt.Println()
		}
		
		if msg.Content != "" {
			pterm.Println(pterm.NewStyle(pterm.FgWhite).Sprint("  " + msg.Content))
		}
		
		if len(msg.ToolCalls) > 0 {
			pterm.Println(pterm.FgYellow.Sprint("  🔧 Tool Calls:"))
			for _, tc := range msg.ToolCalls {
				pterm.Printf("    • %s", pterm.FgYellow.Sprint(tc.Name))
				
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Arguments), &args); err == nil {
					argsStr, _ := json.MarshalIndent(args, "      ", "  ")
					pterm.Println(pterm.FgGray.Sprintf("\n      %s", string(argsStr)))
				} else {
					pterm.Println()
				}
			}
		}
		
	case models.RoleTool:
		pterm.Println(pterm.FgMagenta.Sprintf("🛠️  Tool Result [%s]:", msg.CreatedAt.Format("15:04:05")))
		if msg.ToolResult != nil {
			if msg.ToolResult.IsError {
				pterm.Println(pterm.FgRed.Sprintf("  ❌ %s (Error)", msg.ToolResult.Name))
			} else {
				pterm.Println(pterm.FgGreen.Sprintf("  ✅ %s", msg.ToolResult.Name))
			}
			contentPreview := msg.ToolResult.Content
			if len(contentPreview) > 200 {
				contentPreview = contentPreview[:200] + "..."
			}
			pterm.Println(pterm.FgGray.Sprint("  " + contentPreview))
		}
	}
	
	fmt.Println()
}

func init() {
	rootCmd.AddCommand(chatCmd)
	chatCmd.Flags().String("session", "", "Session ID (creates new if not provided)")
	chatCmd.Flags().String("model", "", "Model ID (required)")
	chatCmd.MarkFlagRequired("model")
}
