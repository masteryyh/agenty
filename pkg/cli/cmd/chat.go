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
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/chat/provider"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/pterm/pterm"
)

func startChat(b backend.Backend) error {
	showBanner()

	if err := runWizardIfNeeded(b); err != nil {
		return err
	}

	agents, err := b.ListAgents(1, 100)
	if err != nil {
		return fmt.Errorf("failed to list agents: %w", err)
	}
	if len(agents.Data) == 0 {
		pterm.Warning.Println("No agents available, creating a default agent...")
		defaultSoul := ""
		_, err := b.CreateAgent(&models.CreateAgentDto{
			Name:      "default",
			Soul:      &defaultSoul,
			IsDefault: true,
		})
		if err != nil {
			return fmt.Errorf("failed to create default agent: %w", err)
		}
		agents, err = b.ListAgents(1, 100)
		if err != nil {
			return fmt.Errorf("failed to list agents: %w", err)
		}
	}

	var agentID uuid.UUID
	var agentName string
	if len(agents.Data) == 1 {
		agentID = agents.Data[0].ID
		agentName = agents.Data[0].Name
	} else {
		var defaultAgent *models.AgentDto
		for i := range agents.Data {
			if agents.Data[i].IsDefault {
				defaultAgent = &agents.Data[i]
				break
			}
		}
		if defaultAgent != nil {
			agentID = defaultAgent.ID
			agentName = defaultAgent.Name
		} else {
			agentNames := make([]string, 0, len(agents.Data))
			for _, a := range agents.Data {
				agentNames = append(agentNames, a.Name)
			}
			selectedName, err := pterm.DefaultInteractiveSelect.
				WithOptions(agentNames).
				WithDefaultText("Select an agent").
				Show()
			if err != nil {
				return fmt.Errorf("agent selection failed: %w", err)
			}
			for _, a := range agents.Data {
				if a.Name == selectedName {
					agentID = a.ID
					agentName = a.Name
					break
				}
			}
		}
	}

	var sessionID uuid.UUID
	var session *models.ChatSessionDto
	var isResumed bool

	lastSession, err := b.GetLastSessionByAgent(agentID)
	if err == nil && lastSession != nil {
		sessionID = lastSession.ID
		session = lastSession
		isResumed = true
	} else {
		if err != nil {
			pterm.Warning.Printf("Could not find last session: %v\n", err)
		}

		session, err = b.CreateSession(agentID)
		if err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
		sessionID = session.ID
	}

	var modelID uuid.UUID
	var modelInfo string
	if session.LastUsedModel == uuid.Nil {
		defaultModel, err := b.GetDefaultModel()
		if err == nil && defaultModel != nil {
			modelID = defaultModel.ID
			modelInfo = modelDisplayName(*defaultModel)
		} else {
			modelsList, err := b.ListModels(1, 1)
			if err != nil {
				return fmt.Errorf("failed to list models: %w", err)
			}
			if len(modelsList.Data) > 0 {
				modelID = modelsList.Data[0].ID
				modelInfo = modelDisplayName(modelsList.Data[0])
			} else {
				return fmt.Errorf("no models available, use /model to create one")
			}
		}
	} else {
		modelID = session.LastUsedModel
		if allModels, err := b.ListModels(1, 100); err == nil {
			for _, m := range allModels.Data {
				if m.ID == modelID {
					modelInfo = modelDisplayName(m)
					break
				}
			}
		}
		if modelInfo == "" {
			modelInfo = modelID.String()[:8] + "…"
		}
	}

	sessionDesc := "new"
	if isResumed {
		sessionDesc = fmt.Sprintf("resumed · %d messages", len(session.Messages))
	}

	fmt.Printf("  %-10s %s\n", pterm.FgGray.Sprint("Agent"), pterm.FgCyan.Sprint(agentName))
	fmt.Printf("  %-10s %s\n", pterm.FgGray.Sprint("Model"), pterm.FgMagenta.Sprint(modelInfo))
	fmt.Printf("  %-10s %s  %s\n", pterm.FgGray.Sprint("Session"),
		pterm.FgGray.Sprint(sessionID.String()[:8]+"…"),
		pterm.FgGray.Sprint("("+sessionDesc+")"))
	fmt.Println()

	if len(session.Messages) > 0 {
		messageCount := len(session.Messages)
		startIdx := 0

		if messageCount > 10 {
			startIdx = messageCount - 10
			fmt.Printf("  %s\n\n", pterm.FgGray.Sprintf("Showing last 10 of %d messages  ·  /history to view all", messageCount))
		}

		fmt.Printf("  %s\n  %s\n\n", pterm.Bold.Sprint("Chat History"), pterm.FgGray.Sprint(strings.Repeat("─", 56)))
		printMessageHistory(session.Messages[startIdx:])
		fmt.Println()
	}

	fmt.Printf("  %s\n\n", pterm.FgGray.Sprintf("Type %s for commands  ·  %s to quit",
		pterm.FgWhite.Sprint("/help"), pterm.FgWhite.Sprint("/exit")))

	return runChatLoop(b, sessionID, modelID, agentID)
}

func runChatLoop(b backend.Backend, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID) error {
	currentSessionID := sessionID
	currentModelID := modelID
	currentAgentID := agentID
	chatState := &ChatState{}
	basePrompt := pterm.FgCyan.Sprint("❯ ")
	var cachedModelNames []string
	var cachedModelAt time.Time

	var cachedAgentNames []string
	var cachedAgentAt time.Time

	agentProvider := func() []string {
		if len(cachedAgentNames) > 0 && time.Since(cachedAgentAt) < 30*time.Second {
			return cachedAgentNames
		}
		agents, err := b.ListAgents(1, 100)
		if err != nil {
			if len(cachedAgentNames) > 0 {
				return cachedAgentNames
			}
			return nil
		}
		names := make([]string, 0, len(agents.Data))
		for _, a := range agents.Data {
			names = append(names, a.Name)
		}
		cachedAgentNames = names
		cachedAgentAt = time.Now()
		return names
	}

	modelProvider := func() []string {
		if len(cachedModelNames) > 0 && time.Since(cachedModelAt) < 30*time.Second {
			return cachedModelNames
		}

		models, err := b.ListModels(1, 100)
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

	thinkLevelProvider := func() []string {
		levels, err := b.GetModelThinkingLevels(currentModelID)
		if err != nil || levels == nil || len(*levels) == 0 {
			return []string{"off"}
		}
		result := make([]string, 0, len(*levels)+1)
		result = append(result, "off")
		result = append(result, *levels...)
		return result
	}
	SetArgCompleter("/think", thinkLevelProvider)
	SetArgCompleter("/model", modelProvider)
	SetArgCompleter("/agent", agentProvider)

	completer := NewChatCompleter()
	painter := NewHintPainter(basePrompt)

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

		painter.ClearInlineHint(len(line), rl)
		rl.Write([]byte("\033[J"))

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		if strings.HasPrefix(input, "/") {
			result, err := handleSlashCommand(b, input, currentSessionID, currentModelID, currentAgentID, chatState)
			if err != nil {
				pterm.Error.Printf("Command error: %v\n", err)
				continue
			}
			if result.Handled {
				if result.ShouldExit {
					break
				}
				if result.NewAgentID != uuid.Nil {
					currentAgentID = result.NewAgentID
					cachedAgentNames = nil
					cachedAgentAt = time.Time{}
				}
				if result.NewSessionID != uuid.Nil {
					currentSessionID = result.NewSessionID
					rl.SetPrompt(basePrompt)
				}
				if result.NewModelID != uuid.Nil {
					currentModelID = result.NewModelID
					cachedModelAt = time.Time{}
					cachedModelNames = nil
				}
				continue
			}

			PrintMatchingCommandHints(input)
			continue
		}

		fmt.Println(pterm.FgGreen.Sprint("🤖 Assistant:"))

		var (
			hasReasoning   bool
			hasToolSection bool
			hadToolCalls   bool
			hasContent     bool
			atLineStart    = true
		)

		err = b.StreamChat(context.Background(), currentSessionID, &models.ChatDto{
			ModelID:       currentModelID,
			Message:       input,
			Thinking:      chatState.Thinking,
			ThinkingLevel: chatState.ThinkingLevel,
		}, func(evt provider.StreamEvent) error {
			switch evt.Type {
			case provider.EventReasoningDelta:
				if !hasReasoning {
					hasReasoning = true
					fmt.Println(pterm.FgLightBlue.Sprint("  💭 Reasoning:"))
					atLineStart = true
				}
				printStreamText(evt.Reasoning, pterm.FgGray, "  ", &atLineStart)

			case provider.EventContentDelta:
				if !hasContent {
					hasContent = true
					if !atLineStart {
						fmt.Println()
						atLineStart = true
					}
					if hadToolCalls {
						fmt.Println()
						fmt.Println(pterm.FgGreen.Sprint("  📝 Final Response:"))
					} else if hasReasoning {
						fmt.Println()
					}
					atLineStart = true
				}
				for i, part := range strings.Split(evt.Content, "\n") {
					if i > 0 {
						fmt.Println()
						atLineStart = true
					}
					if atLineStart && part != "" {
						fmt.Print("  ")
						atLineStart = false
					}
					fmt.Print(part)
				}

			case provider.EventToolCallStart:
				if !hasToolSection {
					if hasContent {
						if !atLineStart {
							fmt.Println()
							atLineStart = true
						}
						fmt.Println()
					} else if hasReasoning {
						if !atLineStart {
							fmt.Println()
							atLineStart = true
						}
						fmt.Println()
					}
					fmt.Println(pterm.FgYellow.Sprint("  🔧 Tool Execution:"))
					hasToolSection = true
					hadToolCalls = true
				}
				name := ""
				if evt.ToolCall != nil {
					name = evt.ToolCall.Name
				}
				fmt.Printf("  • %s ", pterm.FgCyan.Sprint(name))
				atLineStart = false

			case provider.EventToolCallDelta:
				if evt.ToolCall != nil {
					fmt.Print(pterm.FgGray.Sprint(evt.ToolCall.Arguments))
				}

			case provider.EventToolCallDone:
				fmt.Println()
				atLineStart = true

			case provider.EventToolResult:
				if evt.ToolResult != nil {
					if evt.ToolResult.IsError {
						fmt.Printf("    %s\n", pterm.FgRed.Sprint("❌ Error"))
					} else {
						fmt.Printf("    %s\n", pterm.FgGreen.Sprint("✅ Success"))
					}
					preview := evt.ToolResult.Content
					if len(preview) > 100 {
						preview = preview[:97] + "..."
					}
					fmt.Printf("    %s\n", pterm.FgGray.Sprint(preview))
					atLineStart = true
				}

			case provider.EventMessageDone:
				if hasContent {
					if !atLineStart {
						fmt.Println()
						atLineStart = true
					}
				} else if hasReasoning && !atLineStart {
					fmt.Println()
				}

				hasReasoning = false
				hasToolSection = false
				hasContent = false
				atLineStart = true

			case provider.EventError:
				if !atLineStart {
					fmt.Println()
				}
				fmt.Printf("  %s\n", pterm.FgRed.Sprintf("Error: %s", evt.Error))
			}
			return nil
		})

		fmt.Println()

		if err != nil {
			pterm.Error.Printf("Error: %v\n", err)
			continue
		}

		fmt.Println()
	}

	return nil
}

