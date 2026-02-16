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
	"io"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/chzyer/readline"
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

		// Auto-select session if not specified
		var sessionID uuid.UUID
		var session *models.ChatSessionDto
		var err error

		// Try to get the last session
		sessions, err := c.ListSessions(1, 1)
		if err == nil && len(sessions.Data) > 0 {
			session = &sessions.Data[0]
			sessionID = session.ID
			pterm.Info.Printf("Resuming last session: %s\n", sessionID)
		} else {
			// Create new session if no existing sessions
			session, err = c.CreateSession()
			if err != nil {
				return fmt.Errorf("failed to create session: %w", err)
			}
			sessionID = session.ID
			pterm.Info.Printf("Created new session: %s\n", sessionID)
		}

		// Try to get a default model
		modelsList, err := c.ListModels(1, 1)
		if err != nil || len(modelsList.Data) == 0 {
			return fmt.Errorf("no models available, please create a model first or specify --model flag")
		}
		modelID := modelsList.Data[0].ID
		pterm.Info.Printf("Using model: %s/%s\n", modelsList.Data[0].Provider.Name, modelsList.Data[0].Name)

		fmt.Println()

		// Load and display session history if exists
		if len(session.Messages) > 0 {
			// Reload session to get full message details
			session, err = c.GetSession(sessionID)
			if err != nil {
				return fmt.Errorf("failed to reload session: %w", err)
			}
			
			// Display limited history
			const maxInitialMessages = 10
			messageCount := len(session.Messages)
			startIdx := 0
			
			if messageCount > maxInitialMessages {
				startIdx = messageCount - maxInitialMessages
				pterm.Info.Printf("Showing last %d messages (total: %d). Use /history to load more.\n", maxInitialMessages, messageCount)
			}
			
			pterm.DefaultSection.Println("Previous Messages")
			printMessageHistory(session.Messages[startIdx:])
			fmt.Println()
		}

		printChatHelp()
		fmt.Println()

		return runChatLoop(c, sessionID, modelID)
	},
}

func printChatHelp() {
	pterm.Info.Println("Available commands:")
	pterm.Println("  • Type your message and press Enter to chat")
	pterm.Println("  • /new - Start a new chat session")
	pterm.Println("  • /status - Show current session status")
	pterm.Println("  • /history [n] - Load more message history (default: 20)")
	pterm.Println("  • /model [provider/model] - Switch to a different model")
	pterm.Println("  • /exit - Quit the chat")
}

func clearScreen() {
	fmt.Print("\033[2J\033[H")
}

func runChatLoop(c *client.Client, sessionID uuid.UUID, modelID uuid.UUID) error {
	currentSessionID := sessionID
	currentModelID := modelID

	// Create readline completer
	completer := readline.NewPrefixCompleter(
		readline.PcItem("/new"),
		readline.PcItem("/status"),
		readline.PcItem("/history"),
		readline.PcItem("/model"),
		readline.PcItem("/help"),
		readline.PcItem("/exit"),
	)

	// Configure readline
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          pterm.FgCyan.Sprint("You: "),
		HistoryFile:     "/tmp/agenty-chat-history.txt",
		AutoComplete:    completer,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",

		HistorySearchFold: true,
		FuncFilterInputRune: func(r rune) (rune, bool) {
			// Allow all runes including Chinese characters
			return r, true
		},
	})
	if err != nil {
		return fmt.Errorf("failed to initialize readline: %w", err)
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt {
				if len(line) == 0 {
					pterm.Info.Println("Goodbye!")
					break
				} else {
					continue
				}
			} else if err == io.EOF {
				pterm.Info.Println("Goodbye!")
				break
			}
			return fmt.Errorf("readline error: %w", err)
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		// Check for exit
		if strings.ToLower(input) == "/exit" {
			pterm.Info.Println("Goodbye!")
			break
		}

		// Check for slash commands
		if strings.HasPrefix(input, "/") {
			handled, newSessionID, newModelID, err := handleSlashCommand(c, input, currentSessionID)
			if err != nil {
				pterm.Error.Printf("Command error: %v\n", err)
				continue
			}
			if handled {
				// Update current IDs if changed
				if newSessionID != uuid.Nil {
					currentSessionID = newSessionID
					// Update readline prompt with new session indicator
					rl.SetPrompt(pterm.FgCyan.Sprint("You: "))
				}
				if newModelID != uuid.Nil {
					currentModelID = newModelID
				}
				continue
			}
		}

		// Regular chat message
		spinner, _ := pterm.DefaultSpinner.Start("Thinking...")

		messages, err := c.Chat(currentSessionID, &models.ChatDto{
			ModelID: currentModelID,
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
}

func handleSlashCommand(c *client.Client, input string, sessionID uuid.UUID) (handled bool, newSessionID uuid.UUID, newModelID uuid.UUID, err error) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false, uuid.Nil, uuid.Nil, nil
	}

	command := strings.ToLower(parts[0])

	switch command {
	case "/new":
		return handleNewCommand(c)

	case "/status":
		return handleStatusCommand(c, sessionID)
		
	case "/history":
		count := 20
		if len(parts) > 1 {
			if n, err := fmt.Sscanf(parts[1], "%d", &count); err == nil && n == 1 {
				// use parsed count
			}
		}
		return handleHistoryCommand(c, sessionID, count)

	case "/model":
		if len(parts) < 2 {
			return true, uuid.Nil, uuid.Nil, fmt.Errorf("usage: /model [provider-name/model-name]")
		}
		return handleModelCommand(c, parts[1])

	case "/help":
		printChatHelp()
		return true, uuid.Nil, uuid.Nil, nil

	default:
		return false, uuid.Nil, uuid.Nil, nil
	}
}

func handleNewCommand(c *client.Client) (bool, uuid.UUID, uuid.UUID, error) {
	session, err := c.CreateSession()
	if err != nil {
		return true, uuid.Nil, uuid.Nil, fmt.Errorf("failed to create new session: %w", err)
	}

	clearScreen()
	pterm.Success.Printf("Started new session: %s\n", session.ID)
	fmt.Println()
	printChatHelp()
	fmt.Println()

	return true, session.ID, uuid.Nil, nil
}

func handleStatusCommand(c *client.Client, sessionID uuid.UUID) (bool, uuid.UUID, uuid.UUID, error) {
	session, err := c.GetSession(sessionID)
	if err != nil {
		return true, uuid.Nil, uuid.Nil, fmt.Errorf("failed to get session: %w", err)
	}

	fmt.Println()
	pterm.DefaultHeader.Println("📊 Session Status")
	pterm.Printf("  Session ID: %s\n", pterm.FgCyan.Sprint(session.ID))
	pterm.Printf("  Token Consumed: %s\n", pterm.FgYellow.Sprint(session.TokenConsumed))
	pterm.Printf("  Messages: %s\n", pterm.FgGreen.Sprint(len(session.Messages)))
	pterm.Printf("  Created: %s\n", pterm.FgGray.Sprint(session.CreatedAt.Format("2006-01-02 15:04:05")))
	pterm.Printf("  Updated: %s\n", pterm.FgGray.Sprint(session.UpdatedAt.Format("2006-01-02 15:04:05")))
	fmt.Println()

	return true, uuid.Nil, uuid.Nil, nil
}

func handleHistoryCommand(c *client.Client, sessionID uuid.UUID, count int) (bool, uuid.UUID, uuid.UUID, error) {
	session, err := c.GetSession(sessionID)
	if err != nil {
		return true, uuid.Nil, uuid.Nil, fmt.Errorf("failed to get session: %w", err)
	}
	
	messageCount := len(session.Messages)
	if messageCount == 0 {
		pterm.Info.Println("No message history available")
		return true, uuid.Nil, uuid.Nil, nil
	}
	
	// Calculate which messages to show
	startIdx := 0
	if messageCount > count {
		startIdx = messageCount - count
	}
	
	fmt.Println()
	pterm.DefaultHeader.Printf("📜 Message History (%d of %d total)\n", count, messageCount)
	if startIdx > 0 {
		pterm.Info.Printf("Showing messages from #%d to #%d\n", startIdx+1, messageCount)
	}
	fmt.Println()
	
	printMessageHistory(session.Messages[startIdx:])
	fmt.Println()
	
	return true, uuid.Nil, uuid.Nil, nil
}

func handleModelCommand(c *client.Client, modelSpec string) (bool, uuid.UUID, uuid.UUID, error) {
	// Parse provider/model format
	parts := strings.Split(modelSpec, "/")
	if len(parts) != 2 {
		return true, uuid.Nil, uuid.Nil, fmt.Errorf("invalid format, use: provider-name/model-name")
	}

	providerName := strings.TrimSpace(parts[0])
	modelName := strings.TrimSpace(parts[1])

	// Search for provider
	providers, err := c.ListProviders(1, 100)
	if err != nil {
		return true, uuid.Nil, uuid.Nil, fmt.Errorf("failed to list providers: %w", err)
	}

	var providerID uuid.UUID
	for _, p := range providers.Data {
		if strings.EqualFold(p.Name, providerName) {
			providerID = p.ID
			break
		}
	}

	if providerID == uuid.Nil {
		return true, uuid.Nil, uuid.Nil, fmt.Errorf("provider '%s' not found", providerName)
	}

	// Search for model
	modelsList, err := c.ListModels(1, 100)
	if err != nil {
		return true, uuid.Nil, uuid.Nil, fmt.Errorf("failed to list models: %w", err)
	}

	var foundModel *models.ModelDto
	for _, m := range modelsList.Data {
		if m.Provider != nil && m.Provider.ID == providerID && strings.EqualFold(m.Name, modelName) {
			foundModel = &m
			break
		}
	}

	if foundModel == nil {
		return true, uuid.Nil, uuid.Nil, fmt.Errorf("model '%s' not found in provider '%s'", modelName, providerName)
	}

	pterm.Success.Printf("Switched to model: %s (from %s)\n", foundModel.Name, foundModel.Provider.Name)

	return true, uuid.Nil, foundModel.ID, nil
}

// printMessageHistory groups and displays messages, merging tool calling sequences
func printMessageHistory(messages []models.ChatMessageDto) {
	i := 0
	for i < len(messages) {
		msg := &messages[i]
		
		// Check if this is an assistant message with tool calls
		if msg.Role == models.RoleAssistant && len(msg.ToolCalls) > 0 {
			// This is the start of a tool calling sequence
			printToolCallingSequence(messages, &i)
		} else {
			// Regular message
			printMessage(msg)
			i++
		}
	}
}

// printToolCallingSequence displays a complete tool calling sequence in a grouped format
func printToolCallingSequence(messages []models.ChatMessageDto, idx *int) {
	i := *idx
	assistantMsg := &messages[i]
	
	modelInfo := ""
	if assistantMsg.Model != nil {
		modelInfo = fmt.Sprintf(" (%s)", assistantMsg.Model.Name)
	}
	
	// Header for the tool calling sequence
	pterm.Println(pterm.FgGreen.Sprintf("🤖 Assistant%s [%s]:", modelInfo, assistantMsg.CreatedAt.Format("15:04:05")))
	
	// Show reasoning if available
	if assistantMsg.ProviderSpecifics != nil && assistantMsg.ProviderSpecifics.KimiReasoningContent != "" {
		pterm.Println(pterm.FgBlue.Sprint("  💭 Reasoning:"))
		pterm.Println(pterm.NewStyle(pterm.FgGray).Sprint("  " + assistantMsg.ProviderSpecifics.KimiReasoningContent))
		fmt.Println()
	}
	
	// Show initial content if any
	if assistantMsg.Content != "" {
		pterm.Println(pterm.NewStyle(pterm.FgWhite).Sprint("  " + assistantMsg.Content))
		fmt.Println()
	}
	
	// Display tool calls in a tree structure
	pterm.Println(pterm.FgYellow.Sprint("  🔧 Tool Execution:"))
	
	for tcIdx, tc := range assistantMsg.ToolCalls {
		isLast := tcIdx == len(assistantMsg.ToolCalls)-1
		prefix := "  ├─"
		if isLast {
			prefix = "  └─"
		}
		
		pterm.Printf("%s %s ", pterm.FgYellow.Sprint(prefix), pterm.FgCyan.Sprint(tc.Name))
		
		// Parse and display arguments compactly
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err == nil {
			argsStr, _ := json.Marshal(args)
			argDisplay := string(argsStr)
			if len(argDisplay) > 80 {
				argDisplay = argDisplay[:77] + "..."
			}
			pterm.Println(pterm.FgGray.Sprint(argDisplay))
		} else {
			pterm.Println()
		}
		
		// Look for the corresponding tool result
		i++
		if i < len(messages) && messages[i].Role == models.RoleTool && messages[i].ToolResult != nil {
			toolResultMsg := &messages[i]
			resultPrefix := "    "
			if !isLast {
				resultPrefix = "  │ "
			} else {
				resultPrefix = "    "
			}
			
			if toolResultMsg.ToolResult.IsError {
				pterm.Printf("%s%s\n", resultPrefix, pterm.FgRed.Sprint("❌ Error"))
				contentPreview := toolResultMsg.ToolResult.Content
				if len(contentPreview) > 100 {
					contentPreview = contentPreview[:97] + "..."
				}
				pterm.Printf("%s%s\n", resultPrefix, pterm.FgGray.Sprint(contentPreview))
			} else {
				pterm.Printf("%s%s\n", resultPrefix, pterm.FgGreen.Sprint("✅ Success"))
				contentPreview := toolResultMsg.ToolResult.Content
				if len(contentPreview) > 100 {
					contentPreview = contentPreview[:97] + "..."
				}
				pterm.Printf("%s%s\n", resultPrefix, pterm.FgGray.Sprint(contentPreview))
			}
		}
	}
	
	// Move to next message after tool sequence
	i++
	
	// Check if the next message is the assistant's final response
	if i < len(messages) && messages[i].Role == models.RoleAssistant {
		finalMsg := &messages[i]
		if finalMsg.Content != "" {
			fmt.Println()
			pterm.Println(pterm.FgGreen.Sprint("  📝 Final Response:"))
			pterm.Println(pterm.NewStyle(pterm.FgWhite).Sprint("  " + finalMsg.Content))
		}
		i++
	}
	
	fmt.Println()
	*idx = i
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
	chatCmd.Flags().String("session", "", "Session ID (uses last session if not provided)")
	chatCmd.Flags().String("model", "", "Model ID (uses first available model if not provided)")
}
