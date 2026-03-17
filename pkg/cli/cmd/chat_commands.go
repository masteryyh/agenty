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
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/pterm/pterm"
)

type ChatState struct {
	Thinking      bool
	ThinkingLevel string
}

type CommandResult struct {
	Handled      bool
	NewSessionID uuid.UUID
	NewModelID   uuid.UUID
	NewAgentID   uuid.UUID
	ShouldExit   bool
}

type CommandHandler func(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error)

var commandRegistry = map[string]CommandHandler{
	"/new":      handleNewCmd,
	"/status":   handleStatusCmd,
	"/history":  handleHistoryCmd,
	"/model":    handleModelCmd,
	"/think":    handleThinkCmd,
	"/help":     handleHelpCmd,
	"/exit":     handleExitCmd,
	"/agent":    handleAgentCmd,
	"/provider": handleProviderCmd,
	"/mcp":      handleMCPCmd,
}

func parseSlashInput(input string) []string {
	var parts []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	for _, r := range input {
		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case (r == ' ' || r == '\t') && !inSingle && !inDouble:
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func handleSlashCommand(b backend.Backend, input string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	parts := parseSlashInput(input)
	if len(parts) == 0 {
		return CommandResult{}, nil
	}

	command := strings.ToLower(parts[0])

	handler, ok := commandRegistry[command]
	if !ok {
		return CommandResult{}, nil
	}

	return handler(b, parts[1:], sessionID, modelID, agentID, state)
}

func resolveModel(b backend.Backend, modelSpec string) (uuid.UUID, string, error) {
	parts := strings.Split(modelSpec, "/")
	if len(parts) != 2 {
		return uuid.Nil, "", fmt.Errorf("invalid format, use: provider-name/model-name")
	}

	providerName := strings.TrimSpace(parts[0])
	modelName := strings.TrimSpace(parts[1])

	providers, err := b.ListProviders(1, 100)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("failed to list providers: %w", err)
	}

	var providerID uuid.UUID
	for _, p := range providers.Data {
		if strings.EqualFold(p.Name, providerName) {
			providerID = p.ID
			break
		}
	}
	if providerID == uuid.Nil {
		return uuid.Nil, "", fmt.Errorf("provider '%s' not found", providerName)
	}

	modelsList, err := b.ListModels(1, 100)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("failed to list models: %w", err)
	}

	for _, m := range modelsList.Data {
		if m.Provider != nil && m.Provider.ID == providerID && strings.EqualFold(m.Name, modelName) {
			return m.ID, fmt.Sprintf("%s/%s", m.Provider.Name, m.Name), nil
		}
	}

	return uuid.Nil, "", fmt.Errorf("model '%s' not found in provider '%s'", modelName, providerName)
}

func handleExitCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	pterm.Info.Println("Goodbye!")
	return CommandResult{Handled: true, ShouldExit: true}, nil
}

func handleThinkCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	if len(args) == 0 {
		if state.Thinking {
			if state.ThinkingLevel != "" {
				pterm.Info.Printf("Thinking is %s (level: %s)\n", pterm.FgGreen.Sprint("on"), pterm.FgYellow.Sprint(state.ThinkingLevel))
			} else {
				pterm.Info.Printf("Thinking is %s\n", pterm.FgGreen.Sprint("on"))
			}
		} else {
			pterm.Info.Printf("Thinking is %s\n", pterm.FgRed.Sprint("off"))
		}
		pterm.Info.Println("Usage: /think [off|<level>]")
		return CommandResult{Handled: true}, nil
	}

	arg := strings.ToLower(args[0])
	if arg == "off" {
		state.Thinking = false
		state.ThinkingLevel = ""
		pterm.Success.Println("Thinking disabled")
		return CommandResult{Handled: true}, nil
	}

	supportedLevelsPtr, _ := b.GetModelThinkingLevels(modelID)
	var supportedLevels []string
	if supportedLevelsPtr != nil {
		supportedLevels = *supportedLevelsPtr
	}
	valid := false
	for _, l := range supportedLevels {
		if l == arg {
			valid = true
			break
		}
	}
	if !valid {
		if len(supportedLevels) == 0 {
			pterm.Error.Printf("Model does not support thinking\n")
		} else {
			pterm.Error.Printf("Unknown level: %s. Supported: %s\n", arg, strings.Join(supportedLevels, ", "))
		}
		return CommandResult{Handled: true}, nil
	}

	state.Thinking = true
	state.ThinkingLevel = arg
	pterm.Success.Printf("Thinking enabled (level: %s)\n", pterm.FgYellow.Sprint(arg))
	return CommandResult{Handled: true}, nil
}

func handleNewCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	currentSession, err := b.GetSession(sessionID)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to get current session: %w", err)
	}

	session, err := b.CreateSession(currentSession.AgentID)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to create new session: %w", err)
	}

	clearScreen()
	fmt.Printf("  %s  %s\n\n", pterm.FgGray.Sprint("Session"), pterm.FgGray.Sprint(session.ID.String()[:8]+"…  (new)"))
	fmt.Printf("  %s\n\n", pterm.FgGray.Sprintf("Type %s for commands  ·  %s to quit",
		pterm.FgWhite.Sprint("/help"), pterm.FgWhite.Sprint("/exit")))

	return CommandResult{Handled: true, NewSessionID: session.ID}, nil
}

func handleStatusCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	session, err := b.GetSession(sessionID)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to get session: %w", err)
	}

	var currentModelInfo string
	if allModels, err := b.ListModels(1, 100); err == nil {
		for _, m := range allModels.Data {
			if m.ID == modelID {
				if m.Provider != nil {
					currentModelInfo = fmt.Sprintf("%s/%s", m.Provider.Name, m.Name)
				} else {
					currentModelInfo = m.Name
				}
				break
			}
		}
	}
	if currentModelInfo == "" {
		currentModelInfo = modelID.String()
	}

	thinkStatus := pterm.FgGray.Sprint("off")
	if state.Thinking {
		if state.ThinkingLevel != "" {
			thinkStatus = pterm.FgGreen.Sprint("on") + pterm.FgGray.Sprintf("  (%s)", state.ThinkingLevel)
		} else {
			thinkStatus = pterm.FgGreen.Sprint("on")
		}
	}

	fmt.Println()
	fmt.Printf("  %s\n  %s\n\n", pterm.Bold.Sprint("Session Status"), pterm.FgGray.Sprint(strings.Repeat("─", 56)))
	fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Session"), pterm.FgCyan.Sprint(session.ID))
	fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Model"), pterm.FgMagenta.Sprint(currentModelInfo))
	fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Thinking"), thinkStatus)
	fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Tokens used"), pterm.FgYellow.Sprint(session.TokenConsumed))
	fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Messages"), pterm.FgGreen.Sprint(len(session.Messages)))
	fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Created"), pterm.FgGray.Sprint(session.CreatedAt.Format("2006-01-02 15:04:05")))
	fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Updated"), pterm.FgGray.Sprint(session.UpdatedAt.Format("2006-01-02 15:04:05")))
	fmt.Println()

	return CommandResult{Handled: true}, nil
}

func handleHistoryCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	session, err := b.GetSession(sessionID)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to get session: %w", err)
	}

	if len(session.Messages) == 0 {
		pterm.Info.Println("No message history available")
		return CommandResult{Handled: true}, nil
	}

	if err := openHistoryViewer(session.Messages); err != nil {
		pterm.Warning.Printf("Failed to open interactive viewer: %v\n", err)
		pterm.Info.Println("Showing history in console instead...")
		fmt.Println()
		printMessageHistory(session.Messages)
		fmt.Println()
	}

	return CommandResult{Handled: true}, nil
}

func handleHelpCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	PrintCommandHints()
	return CommandResult{Handled: true}, nil
}

func clearScreen() {
	fmt.Print("\033[2J\033[H")
}

var listHints = "↑/↓ navigate  ·  Enter select  ·  a add  ·  e edit  ·  Ctrl+D delete  ·  Esc back"

// --- /model ---

func handleModelCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	if len(args) > 0 {
		resolvedID, displayName, err := resolveModel(b, args[0])
		if err != nil {
			return CommandResult{Handled: true}, err
		}
		pterm.Success.Printf("Switched to model: %s\n", displayName)
		return CommandResult{Handled: true, NewModelID: resolvedID}, nil
	}

	for {
		result, err := b.ListModels(1, 100)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to list models: %w", err)
		}

		if len(result.Data) == 0 {
			pterm.Warning.Println("No models found")
			fmt.Printf("  %s\n", pterm.FgGray.Sprint("Press 'a' to add a model, or Esc to go back"))
			res, err := showInteractiveList("Models", []string{"(no models)"}, "a add  ·  Esc back")
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if res.Action == ListActionAdd {
				if err := doCreateModel(b); err != nil && !errors.Is(err, ErrCancelled) {
					pterm.Error.Printf("Failed to create model: %v\n", err)
				}
				continue
			}
			return CommandResult{Handled: true}, nil
		}

		items := make([]string, len(result.Data))
		for i, m := range result.Data {
			items[i] = modelLabel(m)
		}

		res, err := showInteractiveList("Models  "+pterm.FgGray.Sprint("(select to switch)"), items, listHints)
		if err != nil {
			return CommandResult{Handled: true}, err
		}

		switch res.Action {
		case ListActionSelect:
			target := result.Data[res.Index]
			pterm.Success.Printf("Switched to model: %s\n", modelDisplayName(target))
			return CommandResult{Handled: true, NewModelID: target.ID}, nil

		case ListActionAdd:
			if err := doCreateModel(b); err != nil && !errors.Is(err, ErrCancelled) {
				pterm.Error.Printf("Failed to create model: %v\n", err)
			}
			continue

		case ListActionEdit:
			if err := doUpdateModel(b, result.Data[res.Index]); err != nil && !errors.Is(err, ErrCancelled) {
				pterm.Error.Printf("Failed to update model: %v\n", err)
			}
			continue

		case ListActionDelete:
			target := result.Data[res.Index]
			confirmed, err := showConfirm(fmt.Sprintf("Delete model '%s/%s'?", target.Provider.Name, target.Name))
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if confirmed {
				if err := b.DeleteModel(target.ID); err != nil {
					pterm.Error.Printf("Failed to delete model: %v\n", err)
				} else {
					pterm.Success.Printf("Model deleted: %s/%s\n", target.Provider.Name, target.Name)
				}
			}
			continue

		case ListActionCancel:
			return CommandResult{Handled: true}, nil
		}
	}
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

func doCreateModel(b backend.Backend) error {
	fmt.Print("\033[?25h")
	printSection("Create Model")

	providers, err := b.ListProviders(1, 100)
	if err != nil {
		return fmt.Errorf("failed to list providers: %w", err)
	}
	if len(providers.Data) == 0 {
		pterm.Warning.Println("No providers available. Use /provider to create one first.")
		return nil
	}

	providerOptions := make([]string, len(providers.Data))
	for i, p := range providers.Data {
		providerOptions[i] = providerLabel(p)
	}
	idx, err := selectOption("Select provider", providerOptions, 0)
	if err != nil {
		return err
	}
	selectedProvider := providerOptions[idx]
	var targetProvider models.ModelProviderDto
	for _, p := range providers.Data {
		if providerLabel(p) == selectedProvider {
			targetProvider = p
			break
		}
	}

	name, err := readInput("Model display name", "")
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	code, err := readInput("Model API code/ID", "")
	if err != nil {
		return err
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return fmt.Errorf("code cannot be empty")
	}

	model, err := b.CreateModel(&models.CreateModelDto{
		Name:       name,
		Code:       code,
		ProviderID: targetProvider.ID,
	})
	if err != nil {
		return err
	}
	pterm.Success.Printf("Model created: %s (%s) under %s\n", model.Name, model.Code, targetProvider.Name)
	return nil
}

func doUpdateModel(b backend.Backend, target models.ModelDto) error {
	fmt.Print("\033[?25h")
	printSection("Update Model")

	newName, err := readInput("Model display name", target.Name)
	if err != nil {
		return err
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		newName = target.Name
	}

	setDefault, err := showConfirm("Set as default model?")
	if err != nil {
		return err
	}

	if target.Name == newName && target.DefaultModel == setDefault {
		pterm.Info.Println("No changes detected, skipping update")
		return nil
	}

	if err := b.UpdateModel(target.ID, &models.UpdateModelDto{Name: &newName, DefaultModel: &setDefault}); err != nil {
		return err
	}
	pterm.Success.Printf("Model updated: %s\n", newName)
	return nil
}

// --- /provider ---

var providerTypeOptions = []string{"openai", "anthropic", "gemini", "kimi"}

var providerDefaultBaseURLs = map[string]string{
	"openai":        "https://api.openai.com/v1",
	"openai-legacy": "https://api.openai.com/v1",
	"anthropic":     "https://api.anthropic.com",
	"gemini":        "https://generativelanguage.googleapis.com/v1beta",
	"kimi":          "https://api.moonshot.cn/v1",
}

func providerLabel(p models.ModelProviderDto) string {
	return fmt.Sprintf("%s (%s)", p.Name, string(p.Type))
}

func handleProviderCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	for {
		result, err := b.ListProviders(1, 100)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to list providers: %w", err)
		}

		if len(result.Data) == 0 {
			pterm.Warning.Println("No providers found")
			res, err := showInteractiveList("Providers", []string{"(no providers)"}, "a add  ·  Esc back")
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if res.Action == ListActionAdd {
				if err := doCreateProvider(b); err != nil && !errors.Is(err, ErrCancelled) {
					pterm.Error.Printf("Failed to create provider: %v\n", err)
				}
				continue
			}
			return CommandResult{Handled: true}, nil
		}

		items := make([]string, len(result.Data))
		for i, p := range result.Data {
			items[i] = fmt.Sprintf("%s  %s  %s", providerLabel(p), pterm.FgGray.Sprint(p.BaseURL), pterm.FgGray.Sprint(p.APIKeyCensored))
		}

		res, err := showInteractiveList("Providers", items, listHints)
		if err != nil {
			return CommandResult{Handled: true}, err
		}

		switch res.Action {
		case ListActionSelect:
			target := result.Data[res.Index]
			fmt.Println()
			fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Name"), target.Name)
			fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Type"), string(target.Type))
			fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Base URL"), target.BaseURL)
			fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("API Key"), target.APIKeyCensored)
			fmt.Println()
			continue

		case ListActionAdd:
			if err := doCreateProvider(b); err != nil && !errors.Is(err, ErrCancelled) {
				pterm.Error.Printf("Failed to create provider: %v\n", err)
			}
			continue

		case ListActionEdit:
			if err := doUpdateProvider(b, result.Data[res.Index]); err != nil && !errors.Is(err, ErrCancelled) {
				pterm.Error.Printf("Failed to update provider: %v\n", err)
			}
			continue

		case ListActionDelete:
			target := result.Data[res.Index]
			confirmed, err := showConfirm(fmt.Sprintf("Delete provider '%s' and all its models?", target.Name))
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if confirmed {
				if err := b.DeleteProvider(target.ID, true); err != nil {
					pterm.Error.Printf("Failed to delete provider: %v\n", err)
				} else {
					pterm.Success.Printf("Provider deleted: %s\n", target.Name)
				}
			}
			continue

		case ListActionCancel:
			return CommandResult{Handled: true}, nil
		}
	}
}

func doCreateProvider(b backend.Backend) error {
	fmt.Print("\033[?25h")
	printSection("Create Provider")

	name, err := readInput("Provider name", "")
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	idx, err := selectOption("Provider type", providerTypeOptions, 0)
	if err != nil {
		return err
	}
	selectedType := providerTypeOptions[idx]

	baseURL, err := readInput("Base URL", providerDefaultBaseURLs[selectedType])
	if err != nil {
		return err
	}
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return fmt.Errorf("base URL cannot be empty")
	}

	apiKey, err := readPassword("API key")
	if err != nil {
		return err
	}

	provider, err := b.CreateProvider(&models.CreateModelProviderDto{
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
}

func doUpdateProvider(b backend.Backend, target models.ModelProviderDto) error {
	fmt.Print("\033[?25h")
	printSection("Update Provider")

	newName, err := readInput("Name", target.Name)
	if err != nil {
		return err
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		newName = target.Name
	}

	defaultTypeIdx := 0
	for i, opt := range providerTypeOptions {
		if opt == string(target.Type) {
			defaultTypeIdx = i
			break
		}
	}
	idx, err := selectOption("Provider type", providerTypeOptions, defaultTypeIdx)
	if err != nil {
		return err
	}
	newType := providerTypeOptions[idx]

	newBaseURL, err := readInput("Base URL", target.BaseURL)
	if err != nil {
		return err
	}
	newBaseURL = strings.TrimSpace(newBaseURL)
	if newBaseURL == "" {
		newBaseURL = target.BaseURL
	}

	newAPIKey, err := readPassword("API key (leave empty to keep unchanged)")
	if err != nil {
		return err
	}
	newAPIKey = strings.TrimSpace(newAPIKey)

	if target.Name == newName && string(target.Type) == newType && target.BaseURL == newBaseURL && newAPIKey == "" {
		pterm.Info.Println("No changes detected, skipping update")
		return nil
	}

	updated, err := b.UpdateProvider(target.ID, &models.UpdateModelProviderDto{
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
}

// --- /mcp ---

var mcpTransportOptions = []string{"stdio", "sse", "streamable-http"}

func mcpServerLabel(s models.MCPServerDto) string {
	target := s.URL
	if s.Transport == models.MCPTransportStdio {
		target = s.Command
	}
	return fmt.Sprintf("%s (%s → %s)", s.Name, s.Transport, target)
}

func handleMCPCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	for {
		result, err := b.ListMCPServers(1, 100)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to list MCP servers: %w", err)
		}

		if len(result.Data) == 0 {
			pterm.Warning.Println("No MCP servers found")
			res, err := showInteractiveList("MCP Servers", []string{"(no servers)"}, "a add  ·  Esc back")
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if res.Action == ListActionAdd {
				if err := doCreateMCPServer(b); err != nil && !errors.Is(err, ErrCancelled) {
					pterm.Error.Printf("Failed to register MCP server: %v\n", err)
				}
				continue
			}
			return CommandResult{Handled: true}, nil
		}

		items := make([]string, len(result.Data))
		for i, s := range result.Data {
			statusIcon := "⚪"
			switch s.Status {
			case "connected":
				statusIcon = "🟢"
			case "error":
				statusIcon = "🔴"
			case "connecting":
				statusIcon = "🟡"
			case "disconnected":
				statusIcon = "⚫"
			}
			enabled := "✓"
			if !s.Enabled {
				enabled = "✗"
			}
			items[i] = fmt.Sprintf("%s %s  %s  enabled:%s", statusIcon, mcpServerLabel(s), pterm.FgGray.Sprint(s.Status), enabled)
		}

		res, err := showInteractiveList("MCP Servers", items, listHints)
		if err != nil {
			return CommandResult{Handled: true}, err
		}

		switch res.Action {
		case ListActionSelect:
			target := result.Data[res.Index]
			fmt.Println()
			fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Name"), target.Name)
			fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Transport"), string(target.Transport))
			fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Status"), target.Status)
			if target.Transport == models.MCPTransportStdio {
				fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Command"), target.Command)
			} else {
				fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("URL"), target.URL)
			}
			if len(target.Tools) > 0 {
				fmt.Printf("  %-16s %s\n", pterm.FgGray.Sprint("Tools"), strings.Join(target.Tools, "  ·  "))
			}
			if target.Error != "" {
				fmt.Printf("  %-16s %s\n", pterm.FgRed.Sprint("Error"), target.Error)
			}
			fmt.Println()
			continue

		case ListActionAdd:
			if err := doCreateMCPServer(b); err != nil && !errors.Is(err, ErrCancelled) {
				pterm.Error.Printf("Failed to register MCP server: %v\n", err)
			}
			continue

		case ListActionEdit:
			if err := doUpdateMCPServer(b, result.Data[res.Index]); err != nil && !errors.Is(err, ErrCancelled) {
				pterm.Error.Printf("Failed to update MCP server: %v\n", err)
			}
			continue

		case ListActionDelete:
			target := result.Data[res.Index]
			confirmed, err := showConfirm(fmt.Sprintf("Delete MCP server '%s'?", target.Name))
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if confirmed {
				if err := b.DeleteMCPServer(target.ID); err != nil {
					pterm.Error.Printf("Failed to delete MCP server: %v\n", err)
				} else {
					pterm.Success.Printf("MCP server deleted: %s\n", target.Name)
				}
			}
			continue

		case ListActionCancel:
			return CommandResult{Handled: true}, nil
		}
	}
}

func doCreateMCPServer(b backend.Backend) error {
	fmt.Print("\033[?25h")
	printSection("Register MCP Server")

	name, err := readInput("Server name", "")
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	idx, err := selectOption("Transport type", mcpTransportOptions, 0)
	if err != nil {
		return err
	}
	selectedTransport := mcpTransportOptions[idx]

	dto := &models.CreateMCPServerDto{
		Name:      name,
		Transport: models.MCPTransportType(selectedTransport),
	}

	switch models.MCPTransportType(selectedTransport) {
	case models.MCPTransportStdio:
		command, err := readInput("Command (e.g., npx, node, python)", "")
		if err != nil {
			return err
		}
		dto.Command = strings.TrimSpace(command)

		argsStr, err := readInput("Arguments (space-separated, leave empty for none)", "")
		if err != nil {
			return err
		}
		argsStr = strings.TrimSpace(argsStr)
		if argsStr != "" {
			dto.Args = strings.Fields(argsStr)
		}

	case models.MCPTransportSSE, models.MCPTransportStreamableHTTP:
		url, err := readInput("Server URL", "")
		if err != nil {
			return err
		}
		dto.URL = strings.TrimSpace(url)
	}

	server, err := b.CreateMCPServer(dto)
	if err != nil {
		return err
	}
	pterm.Success.Printf("MCP server registered: %s\n", server.Name)

	autoConnect, err := showConfirm("Connect now?")
	if err != nil {
		return err
	}
	if autoConnect {
		if err := b.ConnectMCPServer(server.ID); err != nil {
			pterm.Warning.Printf("Connection failed: %s\n", err)
		} else {
			pterm.Success.Println("Connected")
		}
	}

	return nil
}

func doUpdateMCPServer(b backend.Backend, target models.MCPServerDto) error {
	fmt.Print("\033[?25h")
	printSection("Update MCP Server")

	newName, err := readInput("Name", target.Name)
	if err != nil {
		return err
	}
	newName = strings.TrimSpace(newName)

	defaultEnabledIdx := 1
	if target.Enabled {
		defaultEnabledIdx = 0
	}
	enabledIdx, err := selectOption("Enabled", []string{"true", "false"}, defaultEnabledIdx)
	if err != nil {
		return err
	}
	enabled := enabledIdx == 0

	dto := &models.UpdateMCPServerDto{
		Name:    newName,
		Enabled: &enabled,
	}

	switch target.Transport {
	case models.MCPTransportStdio:
		newCmd, err := readInput("Command", target.Command)
		if err != nil {
			return err
		}
		dto.Command = strings.TrimSpace(newCmd)
	case models.MCPTransportSSE, models.MCPTransportStreamableHTTP:
		newURL, err := readInput("URL", target.URL)
		if err != nil {
			return err
		}
		dto.URL = strings.TrimSpace(newURL)
	}

	updated, err := b.UpdateMCPServer(target.ID, dto)
	if err != nil {
		return err
	}
	pterm.Success.Printf("MCP server updated: %s\n", updated.Name)
	return nil
}

// --- /agent ---

func handleAgentCmd(b backend.Backend, args []string, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, state *ChatState) (CommandResult, error) {
	if len(args) > 0 {
		return switchToAgent(b, args[0])
	}

	for {
		agents, err := b.ListAgents(1, 100)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to list agents: %w", err)
		}

		if len(agents.Data) == 0 {
			pterm.Warning.Println("No agents found")
			res, err := showInteractiveList("Agents", []string{"(no agents)"}, "a add  ·  Esc back")
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if res.Action == ListActionAdd {
				if err := doCreateAgent(b); err != nil && !errors.Is(err, ErrCancelled) {
					pterm.Error.Printf("Failed to create agent: %v\n", err)
				}
				continue
			}
			return CommandResult{Handled: true}, nil
		}

		items := make([]string, len(agents.Data))
		for i, a := range agents.Data {
			marker := ""
			if a.IsDefault {
				marker = " [default]"
			}
			current := ""
			if a.ID == agentID {
				current = " ← current"
			}
			items[i] = fmt.Sprintf("%s%s%s", a.Name, marker, pterm.FgGray.Sprint(current))
		}

		res, err := showInteractiveList("Agents  "+pterm.FgGray.Sprint("(select to switch)"), items, listHints)
		if err != nil {
			return CommandResult{Handled: true}, err
		}

		switch res.Action {
		case ListActionSelect:
			target := agents.Data[res.Index]
			return switchToAgent(b, target.Name)

		case ListActionAdd:
			if err := doCreateAgent(b); err != nil && !errors.Is(err, ErrCancelled) {
				pterm.Error.Printf("Failed to create agent: %v\n", err)
			}
			continue

		case ListActionEdit:
			if err := doUpdateAgent(b, agents.Data[res.Index]); err != nil && !errors.Is(err, ErrCancelled) {
				pterm.Error.Printf("Failed to update agent: %v\n", err)
			}
			continue

		case ListActionDelete:
			target := agents.Data[res.Index]
			confirmed, err := showConfirm(fmt.Sprintf("Delete agent '%s'? This will also delete all sessions, messages and memories.", target.Name))
			if err != nil {
				return CommandResult{Handled: true}, err
			}
			if confirmed {
				if err := b.DeleteAgent(target.ID); err != nil {
					pterm.Error.Printf("Failed to delete agent: %v\n", err)
				} else {
					pterm.Success.Printf("Agent deleted: %s\n", target.Name)
				}
			}
			continue

		case ListActionCancel:
			return CommandResult{Handled: true}, nil
		}
	}
}

func switchToAgent(b backend.Backend, agentName string) (CommandResult, error) {
	agentName = strings.TrimSpace(agentName)
	agents, err := b.ListAgents(1, 100)
	if err != nil {
		return CommandResult{Handled: true}, fmt.Errorf("failed to list agents: %w", err)
	}

	var targetAgentID uuid.UUID
	for _, a := range agents.Data {
		if strings.EqualFold(a.Name, agentName) {
			targetAgentID = a.ID
			break
		}
	}
	if targetAgentID == uuid.Nil {
		return CommandResult{Handled: true}, fmt.Errorf("agent '%s' not found", agentName)
	}

	lastSession, err := b.GetLastSessionByAgent(targetAgentID)
	var newSessionID uuid.UUID
	if err == nil && lastSession != nil {
		newSessionID = lastSession.ID
		sessionDesc := fmt.Sprintf("resumed · %d messages", len(lastSession.Messages))
		clearScreen()
		fmt.Printf("  %-10s %s\n", pterm.FgGray.Sprint("Agent"), pterm.FgCyan.Sprint(agentName))
		fmt.Printf("  %-10s %s  %s\n\n", pterm.FgGray.Sprint("Session"),
			pterm.FgGray.Sprint(newSessionID.String()[:8]+"…"),
			pterm.FgGray.Sprint("("+sessionDesc+")"))

		if len(lastSession.Messages) > 0 {
			messageCount := len(lastSession.Messages)
			startIdx := 0
			if messageCount > 10 {
				startIdx = messageCount - 10
				fmt.Printf("  %s\n\n", pterm.FgGray.Sprintf("Showing last 10 of %d messages  ·  /history to view all", messageCount))
			}
			fmt.Printf("  %s\n  %s\n\n", pterm.Bold.Sprint("Chat History"), pterm.FgGray.Sprint(strings.Repeat("─", 56)))
			printMessageHistory(lastSession.Messages[startIdx:])
			fmt.Println()
		}
	} else {
		newSession, err := b.CreateSession(targetAgentID)
		if err != nil {
			return CommandResult{Handled: true}, fmt.Errorf("failed to create session for agent: %w", err)
		}
		newSessionID = newSession.ID
		clearScreen()
		fmt.Printf("  %-10s %s\n", pterm.FgGray.Sprint("Agent"), pterm.FgCyan.Sprint(agentName))
		fmt.Printf("  %-10s %s  %s\n\n", pterm.FgGray.Sprint("Session"),
			pterm.FgGray.Sprint(newSessionID.String()[:8]+"…"),
			pterm.FgGray.Sprint("(new)"))
	}

	fmt.Printf("  %s\n\n", pterm.FgGray.Sprintf("Type %s for commands  ·  %s to quit",
		pterm.FgWhite.Sprint("/help"), pterm.FgWhite.Sprint("/exit")))

	return CommandResult{Handled: true, NewAgentID: targetAgentID, NewSessionID: newSessionID}, nil
}

func doCreateAgent(b backend.Backend) error {
	fmt.Print("\033[?25h")
	printSection("Create Agent")

	name, err := readInput("Agent name", "")
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	soul, err := readInput("Agent soul (system prompt, leave empty for default)", "")
	if err != nil {
		return err
	}
	soul = strings.TrimSpace(soul)

	isDefault, err := showConfirm("Set as default agent?")
	if err != nil {
		return err
	}

	agent, err := b.CreateAgent(&models.CreateAgentDto{
		Name:      name,
		Soul:      &soul,
		IsDefault: isDefault,
	})
	if err != nil {
		return err
	}
	pterm.Success.Printf("Agent created: %s (%s)\n", agent.Name, agent.ID)
	return nil
}

func doUpdateAgent(b backend.Backend, target models.AgentDto) error {
	fmt.Print("\033[?25h")
	printSection("Update Agent")

	newName, err := readInput("Agent name", target.Name)
	if err != nil {
		return err
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		newName = target.Name
	}

	newSoul, err := readInput("Agent soul (system prompt)", target.Soul)
	if err != nil {
		return err
	}
	newSoul = strings.TrimSpace(newSoul)
	if newSoul == "" {
		newSoul = target.Soul
	}

	newIsDefault, err := showConfirm("Set as default agent?")
	if err != nil {
		return err
	}

	if newName == target.Name && newSoul == target.Soul && newIsDefault == target.IsDefault {
		pterm.Info.Println("No changes detected, skipping update")
		return nil
	}

	if err := b.UpdateAgent(target.ID, &models.UpdateAgentDto{
		Name:      &newName,
		Soul:      &newSoul,
		IsDefault: &newIsDefault,
	}); err != nil {
		return err
	}
	pterm.Success.Printf("Agent updated: %s\n", newName)
	return nil
}
