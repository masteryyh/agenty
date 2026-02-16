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
	"os"
	"path/filepath"
	"strings"
	"time"

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
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.NewClient(GetBaseURL())

		var sessionID uuid.UUID
		var session *models.ChatSessionDto
		var err error

		sessionFlag, _ := cmd.Flags().GetString("session")
		if sessionFlag != "" {
			sessionID, err = uuid.Parse(sessionFlag)
			if err != nil {
				return fmt.Errorf("invalid session ID: %w", err)
			}
			session, err = c.GetSession(sessionID)
			if err != nil {
				return fmt.Errorf("failed to get session: %w", err)
			}
			pterm.Info.Printf("Using session: %s\n", sessionID)
		} else {
			sessions, listErr := c.ListSessions(1, 1)
			if listErr == nil && len(sessions.Data) > 0 {
				sessionID = sessions.Data[0].ID
				session, err = c.GetSession(sessionID)
				if err != nil {
					return fmt.Errorf("failed to get session details: %w", err)
				}
				pterm.Info.Printf("Resuming last session: %s\n", sessionID)
			} else {
				session, err = c.CreateSession()
				if err != nil {
					return fmt.Errorf("failed to create session: %w", err)
				}
				sessionID = session.ID
				pterm.Info.Printf("Created new session: %s\n", sessionID)
			}
		}

		var modelID uuid.UUID
		modelFlag, _ := cmd.Flags().GetString("model")
		if modelFlag != "" {
			resolvedID, displayName, resolveErr := resolveModel(c, modelFlag)
			if resolveErr != nil {
				return fmt.Errorf("failed to resolve model: %w", resolveErr)
			}
			modelID = resolvedID
			pterm.Info.Printf("Using model: %s\n", displayName)
		} else {
			modelsList, listErr := c.ListModels(1, 1)
			if listErr != nil || len(modelsList.Data) == 0 {
				return fmt.Errorf("no models available, please create a model first or specify --model flag")
			}
			modelID = modelsList.Data[0].ID
			modelInfo := modelsList.Data[0].Name
			if modelsList.Data[0].Provider != nil {
				modelInfo = fmt.Sprintf("%s/%s", modelsList.Data[0].Provider.Name, modelsList.Data[0].Name)
			}
			pterm.Info.Printf("Using model: %s\n", modelInfo)
		}

		fmt.Println()

		if len(session.Messages) > 0 {
			const maxInitialMessages = 10
			messageCount := len(session.Messages)
			startIdx := 0

			if messageCount > maxInitialMessages {
				startIdx = messageCount - maxInitialMessages
				pterm.Info.Printf("Showing last %d messages (total: %d). Use /history to view all.\n", maxInitialMessages, messageCount)
			}

			pterm.DefaultSection.Println("Previous Messages")
			printMessageHistory(session.Messages[startIdx:])
			fmt.Println()
		}

		PrintCommandHints()
		fmt.Println()

		return runChatLoop(c, sessionID, modelID)
	},
}

func runChatLoop(c *client.Client, sessionID uuid.UUID, modelID uuid.UUID) error {
	currentSessionID := sessionID
	currentModelID := modelID
	basePrompt := pterm.FgCyan.Sprint("You: ")
	var cachedModelNames []string
	var cachedModelAt time.Time

	modelProvider := func() []string {
		if len(cachedModelNames) > 0 && time.Since(cachedModelAt) < 30*time.Second {
			return cachedModelNames
		}

		models, err := c.ListModels(1, 100)
		if err != nil {
			if len(cachedModelNames) > 0 {
				return cachedModelNames
			}
			return nil
		}

		modelNames := make([]string, 0, len(models.Data))
		for _, m := range models.Data {
			modelNames = append(modelNames, fmt.Sprintf("%s/%s", m.Provider.Name, m.Name))
		}

		cachedModelNames = modelNames
		cachedModelAt = time.Now()
		return modelNames
	}

	SetArgCompleter("/model", modelProvider)

	completer := NewChatCompleter()
	painter := NewHintPainter()

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}

	config := &readline.Config{
		Prompt:              basePrompt,
		HistoryFile:         filepath.Join(cacheDir, "agenty-chat-history.txt"),
		AutoComplete:        completer,
		Painter:             painter,
		InterruptPrompt:     "^C",
		EOFPrompt:           "exit",
		HistorySearchFold:   true,
		FuncFilterInputRune: func(r rune) (rune, bool) { return r, true },
	}

	rl, err := readline.NewEx(config)
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
				}
				continue
			} else if err == io.EOF {
				pterm.Info.Println("Goodbye!")
				break
			}
			return fmt.Errorf("readline error: %w", err)
		}

		rl.Write([]byte("\033[J"))

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		if strings.ToLower(input) == "/exit" {
			pterm.Info.Println("Goodbye!")
			break
		}

		if strings.HasPrefix(input, "/") {
			handled, newSessionID, newModelID, err := handleSlashCommand(c, input, currentSessionID, currentModelID)
			if err != nil {
				pterm.Error.Printf("Command error: %v\n", err)
				continue
			}
			if handled {
				if newSessionID != uuid.Nil {
					currentSessionID = newSessionID
					rl.SetPrompt(basePrompt)
				}
				if newModelID != uuid.Nil {
					currentModelID = newModelID
					cachedModelAt = time.Time{}
					cachedModelNames = nil
				}
				continue
			}

			PrintMatchingCommandHints(input)
			continue
		}

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

func init() {
	rootCmd.AddCommand(chatCmd)
	chatCmd.Flags().String("session", "", "Session ID to resume")
	chatCmd.Flags().String("model", "", "Model to use (provider/model format)")
}
