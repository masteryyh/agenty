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
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/models"
)

type settingsEditorMode uint8

const (
	settingsEditorModeSelect settingsEditorMode = iota
	settingsEditorModeAPIKeyInput
	settingsEditorModeFirecrawlURLInput
	settingsEditorModeAgentModelsSelect
	settingsEditorModeEmbeddingModelSelect
	settingsEditorModeEmbeddingConfirm
	settingsEditorModeSaving
)

const (
	settingsWebSearchProviderCount = 3
	settingsRowProviderStart       = 0
	settingsRowAgentModels         = settingsWebSearchProviderCount
	settingsRowEmbedding           = settingsWebSearchProviderCount + 1
)

type settingsEditorSaveMsg struct {
	settings *models.SystemSettingsDto
	err      error
	message  string
}

type settingsEditorModelDataMsg struct {
	chatModels           []models.ModelDto
	embeddingModels      []models.ModelDto
	currentAgentModelIDs []uuid.UUID
	err                  error
}

type settingsEditorAgentModelsSavedMsg struct {
	modelIDs []uuid.UUID
	err      error
}

type settingsEditorOverlay struct {
	backend               backend.Backend
	settings              *models.SystemSettingsDto
	mode                  settingsEditorMode
	selection             selectionModel
	agentSelection        selectionModel
	embeddingSelection    selectionModel
	input                 wizInput
	feedback              wizFeedback
	responseCh            chan overlayResponse
	editingProvider       models.WebSearchProvider
	pendingAPIKey         string
	pendingEmbeddingModel *models.ModelDto
	savingLabel           string
	chatModels            []models.ModelDto
	embeddingModels       []models.ModelDto
	currentAgentModelIDs  []uuid.UUID
	modelDataLoaded       bool
}

func newSettingsEditorOverlay(backend backend.Backend, settings *models.SystemSettingsDto, responseCh chan overlayResponse) *settingsEditorOverlay {
	overlay := &settingsEditorOverlay{
		backend:         backend,
		settings:        settings,
		mode:            settingsEditorModeSelect,
		selection:       newSelectionModel(settingsRowEmbedding),
		responseCh:      responseCh,
		savingLabel:     "Loading model settings…",
		modelDataLoaded: false,
	}
	overlay.syncSelection()
	return overlay
}

func (o *settingsEditorOverlay) init() tea.Cmd {
	return o.loadModelData()
}

func (o *settingsEditorOverlay) update(msg tea.Msg) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return o.handleKey(msg)
	case settingsEditorModelDataMsg:
		if msg.err != nil {
			o.feedback = o.feedback.setErr("Failed to load model settings: " + msg.err.Error())
			o.modelDataLoaded = false
			o.mode = settingsEditorModeSelect
			return false, nil
		}
		o.chatModels = msg.chatModels
		o.embeddingModels = msg.embeddingModels
		o.currentAgentModelIDs = msg.currentAgentModelIDs
		o.modelDataLoaded = true
		o.syncModelSelections()
		if o.mode == settingsEditorModeSaving {
			o.mode = settingsEditorModeSelect
		}
		return false, nil
	case settingsEditorAgentModelsSavedMsg:
		if msg.err != nil {
			o.feedback = o.feedback.setErr("Failed to update agent models: " + msg.err.Error())
			o.mode = settingsEditorModeSelect
			return false, nil
		}
		o.currentAgentModelIDs = append([]uuid.UUID(nil), msg.modelIDs...)
		o.feedback = o.feedback.setOK("Agent models updated successfully")
		o.mode = settingsEditorModeSelect
		o.syncModelSelections()
		return false, nil
	case settingsEditorSaveMsg:
		if msg.err != nil {
			o.feedback = o.feedback.setErr("Failed to update settings: " + msg.err.Error())
			o.mode = settingsEditorModeSelect
			return false, nil
		}
		o.settings = msg.settings
		if msg.message != "" {
			o.feedback = o.feedback.setOK(msg.message)
		} else {
			o.feedback = o.feedback.setOK("Settings updated successfully")
		}
		o.mode = settingsEditorModeSelect
		o.syncSelection()
		o.syncModelSelections()
		return false, nil
	default:
		if o.mode == settingsEditorModeAPIKeyInput || o.mode == settingsEditorModeFirecrawlURLInput {
			var cmd tea.Cmd
			o.input, cmd = o.input.Update(msg)
			return false, cmd
		}
	}
	return false, nil
}

func (o *settingsEditorOverlay) handleKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch o.mode {
	case settingsEditorModeSelect:
		if selection, ok := o.selection.HandleNavKey(msg); ok {
			o.selection = selection
			o.feedback = o.feedback.clear()
			return false, nil
		}
		switch msg.Type {
		case tea.KeyEsc, tea.KeyCtrlC:
			if o.responseCh != nil {
				o.responseCh <- overlayResponse{formSubmitted: true}
			}
			return true, nil
		case tea.KeySpace:
			if o.selection.pos < settingsRowAgentModels {
				return false, o.savePrimaryProvider()
			}
		case tea.KeyRunes:
			if string(msg.Runes) == " " && o.selection.pos < settingsRowAgentModels {
				return false, o.savePrimaryProvider()
			}
		case tea.KeyEnter:
			switch o.selection.pos {
			case settingsRowAgentModels:
				if !o.modelDataLoaded {
					o.feedback = o.feedback.setWarn("Model data is still loading")
					return false, nil
				}
				if len(o.chatModels) == 0 {
					o.feedback = o.feedback.setWarn("No chat models from configured providers found")
					return false, nil
				}
				o.mode = settingsEditorModeAgentModelsSelect
				o.feedback = o.feedback.clear()
				return false, nil
			case settingsRowEmbedding:
				if !o.modelDataLoaded {
					o.feedback = o.feedback.setWarn("Model data is still loading")
					return false, nil
				}
				if len(o.embeddingModels) == 0 {
					o.feedback = o.feedback.setWarn("No embedding models from configured providers found")
					return false, nil
				}
				o.mode = settingsEditorModeEmbeddingModelSelect
				o.feedback = o.feedback.clear()
				return false, nil
			default:
				return false, o.beginProviderConfig()
			}
		}
	case settingsEditorModeAPIKeyInput:
		switch msg.Type {
		case tea.KeyEsc:
			o.mode = settingsEditorModeSelect
			o.feedback = o.feedback.clear()
			return false, nil
		case tea.KeyEnter:
			key := o.input.Value()
			if key == "" {
				o.feedback = o.feedback.setWarn("API key cannot be empty")
				return false, nil
			}
			o.pendingAPIKey = key
			if o.editingProvider == models.WebSearchProviderFirecrawl {
				input, cmd := o.input.Reset("Base URL", false)
				input.model.SetValue(firstNonEmptyString(o.settings.FirecrawlBaseURL, "https://api.firecrawl.dev"))
				o.input = input
				o.mode = settingsEditorModeFirecrawlURLInput
				o.feedback = o.feedback.clear()
				return false, cmd
			}
			return false, o.saveProviderConfig(o.editingProvider, key, "")
		}
	case settingsEditorModeFirecrawlURLInput:
		switch msg.Type {
		case tea.KeyEsc:
			o.mode = settingsEditorModeSelect
			o.feedback = o.feedback.clear()
			return false, nil
		case tea.KeyEnter:
			return false, o.saveProviderConfig(models.WebSearchProviderFirecrawl, o.pendingAPIKey, o.input.Value())
		}
	case settingsEditorModeAgentModelsSelect:
		if selection, ok := o.agentSelection.HandleNavKey(msg); ok {
			o.agentSelection = selection
			return false, nil
		}
		switch msg.Type {
		case tea.KeyEsc:
			o.mode = settingsEditorModeSelect
			o.feedback = o.feedback.clear()
			return false, nil
		case tea.KeySpace:
			o.agentSelection = o.agentSelection.toggle(o.agentSelection.pos)
			return false, nil
		case tea.KeyRunes:
			if string(msg.Runes) == " " {
				o.agentSelection = o.agentSelection.toggle(o.agentSelection.pos)
				return false, nil
			}
		case tea.KeyEnter:
			if len(o.agentSelection.selected) == 0 {
				o.feedback = o.feedback.setWarn("Select at least one chat model")
				return false, nil
			}
			return false, o.saveAgentModels()
		}
	case settingsEditorModeEmbeddingModelSelect:
		if selection, ok := o.embeddingSelection.HandleNavKey(msg); ok {
			o.embeddingSelection = selection
			return false, nil
		}
		switch msg.Type {
		case tea.KeyEsc:
			o.mode = settingsEditorModeSelect
			o.feedback = o.feedback.clear()
			return false, nil
		case tea.KeyEnter:
			chosen := o.embeddingModels[o.embeddingSelection.pos]
			if o.settings.EmbeddingModelID != nil && *o.settings.EmbeddingModelID != chosen.ID {
				o.pendingEmbeddingModel = &chosen
				o.mode = settingsEditorModeEmbeddingConfirm
				return false, nil
			}
			return false, o.saveEmbeddingModel(chosen.ID)
		}
	case settingsEditorModeEmbeddingConfirm:
		switch msg.Type {
		case tea.KeyEsc:
			o.pendingEmbeddingModel = nil
			o.mode = settingsEditorModeEmbeddingModelSelect
			return false, nil
		case tea.KeyEnter:
			if o.pendingEmbeddingModel == nil {
				o.mode = settingsEditorModeEmbeddingModelSelect
				return false, nil
			}
			modelID := o.pendingEmbeddingModel.ID
			o.pendingEmbeddingModel = nil
			return false, o.saveEmbeddingModel(modelID)
		}
	case settingsEditorModeSaving:
		if msg.Type == tea.KeyEsc || msg.Type == tea.KeyCtrlC {
			return false, nil
		}
	}
	return false, nil
}

func (o *settingsEditorOverlay) beginProviderConfig() tea.Cmd {
	o.editingProvider = webSearchProviders[o.selection.pos]
	input, cmd := o.input.Reset("API Key", false)
	o.input = input
	o.feedback = o.feedback.clear()
	o.mode = settingsEditorModeAPIKeyInput
	return cmd
}

func (o *settingsEditorOverlay) savePrimaryProvider() tea.Cmd {
	provider := webSearchProviders[o.selection.pos]
	if !o.providerConfigured(provider) {
		o.feedback = o.feedback.setWarn("Configure this provider before setting it as primary")
		return nil
	}

	o.mode = settingsEditorModeSaving
	o.savingLabel = "Saving primary provider…"
	backend := o.backend
	return func() tea.Msg {
		updated, err := backend.UpdateSystemSettings(&models.UpdateSystemSettingsDto{
			WebSearchProvider: &provider,
		})
		return settingsEditorSaveMsg{settings: updated, err: err, message: "Primary web search provider updated"}
	}
}

func (o *settingsEditorOverlay) saveProviderConfig(provider models.WebSearchProvider, apiKey, baseURL string) tea.Cmd {
	dto := &models.UpdateSystemSettingsDto{}
	switch provider {
	case models.WebSearchProviderTavily:
		dto.TavilyAPIKey = &apiKey
	case models.WebSearchProviderBrave:
		dto.BraveAPIKey = &apiKey
	case models.WebSearchProviderFirecrawl:
		dto.FirecrawlAPIKey = &apiKey
		dto.FirecrawlBaseURL = &baseURL
	}

	o.mode = settingsEditorModeSaving
	o.savingLabel = "Saving provider config…"
	backend := o.backend
	return func() tea.Msg {
		updated, err := backend.UpdateSystemSettings(dto)
		return settingsEditorSaveMsg{settings: updated, err: err, message: "Web search provider config updated"}
	}
}

func (o *settingsEditorOverlay) saveAgentModels() tea.Cmd {
	modelIDs := make([]uuid.UUID, len(o.agentSelection.selected))
	for i, idx := range o.agentSelection.selected {
		modelIDs[i] = o.chatModels[idx].ID
	}

	o.mode = settingsEditorModeSaving
	o.savingLabel = "Saving agent models…"
	backend := o.backend
	return func() tea.Msg {
		agents, err := backend.ListAgents(1, 100)
		if err != nil {
			return settingsEditorAgentModelsSavedMsg{err: err}
		}
		if agent := findDefaultAgent(agents.Data); agent != nil {
			err = backend.UpdateAgent(agent.ID, &models.UpdateAgentDto{ModelIDs: &modelIDs})
		} else {
			soul := ""
			_, err = backend.CreateAgent(&models.CreateAgentDto{
				Name:      "default",
				Soul:      &soul,
				IsDefault: true,
				ModelIDs:  modelIDs,
			})
		}
		return settingsEditorAgentModelsSavedMsg{modelIDs: modelIDs, err: err}
	}
}

func (o *settingsEditorOverlay) saveEmbeddingModel(modelID uuid.UUID) tea.Cmd {
	o.mode = settingsEditorModeSaving
	o.savingLabel = "Saving embedding model…"
	backend := o.backend
	return func() tea.Msg {
		updated, err := backend.UpdateSystemSettings(&models.UpdateSystemSettingsDto{EmbeddingModelID: &modelID})
		return settingsEditorSaveMsg{settings: updated, err: err, message: "Active embedding model updated"}
	}
}

func (o *settingsEditorOverlay) loadModelData() tea.Cmd {
	backend := o.backend
	return func() tea.Msg {
		modelResult, err := backend.ListModels(1, 500)
		if err != nil {
			return settingsEditorModelDataMsg{err: fmt.Errorf("failed to list models: %w", err)}
		}
		agentResult, err := backend.ListAgents(1, 100)
		if err != nil {
			return settingsEditorModelDataMsg{err: fmt.Errorf("failed to list agents: %w", err)}
		}

		var chatModels []models.ModelDto
		var embeddingModels []models.ModelDto
		for _, model := range modelResult.Data {
			if model.Provider == nil || model.Provider.APIKeyCensored == "<not set>" {
				continue
			}
			if model.EmbeddingModel {
				embeddingModels = append(embeddingModels, model)
				continue
			}
			chatModels = append(chatModels, model)
		}

		var currentAgentModelIDs []uuid.UUID
		if agent := findDefaultAgent(agentResult.Data); agent != nil {
			currentAgentModelIDs = make([]uuid.UUID, len(agent.Models))
			for i, model := range agent.Models {
				currentAgentModelIDs[i] = model.ID
			}
		}

		return settingsEditorModelDataMsg{
			chatModels:           chatModels,
			embeddingModels:      embeddingModels,
			currentAgentModelIDs: currentAgentModelIDs,
		}
	}
}

func (o *settingsEditorOverlay) providerConfigured(provider models.WebSearchProvider) bool {
	if o.settings == nil {
		return false
	}
	for _, configured := range o.settings.ConfiguredWebSearchProviders {
		if configured == provider {
			return true
		}
	}
	return false
}

func (o *settingsEditorOverlay) syncSelection() {
	if o.settings == nil {
		return
	}
	cursor := 0
	for i, provider := range webSearchProviders {
		if provider == o.settings.WebSearchProvider {
			cursor = i
			break
		}
	}
	o.selection = o.selection.withCursor(cursor)
}

func (o *settingsEditorOverlay) syncModelSelections() {
	defaultAgentIndices := make([]int, 0, len(o.currentAgentModelIDs))
	idToIdx := make(map[uuid.UUID]int, len(o.chatModels))
	for i, model := range o.chatModels {
		idToIdx[model.ID] = i
	}
	for _, id := range o.currentAgentModelIDs {
		if idx, ok := idToIdx[id]; ok {
			defaultAgentIndices = append(defaultAgentIndices, idx)
		}
	}
	o.agentSelection = newSelectionModel(max(0, len(o.chatModels)-1)).withMultiSelect(defaultAgentIndices, wizMaxModels)

	embedCursor := 0
	if o.settings != nil && o.settings.EmbeddingModelID != nil {
		for i, model := range o.embeddingModels {
			if model.ID == *o.settings.EmbeddingModelID {
				embedCursor = i
				break
			}
		}
	}
	o.embeddingSelection = newSelectionModel(max(0, len(o.embeddingModels)-1)).withCursor(embedCursor)
}

func (o *settingsEditorOverlay) render(width, _ int) string {
	var buf strings.Builder
	sep := max(min(56, width-4), 10)
	fmt.Fprintf(&buf, "\n  %s\n", styleBold.Render("System Settings"))
	fmt.Fprintf(&buf, "  %s\n\n", styleGray.Render(strings.Repeat("─", sep)))

	o.renderSelectBlocks(&buf)

	switch o.mode {
	case settingsEditorModeAPIKeyInput, settingsEditorModeFirecrawlURLInput:
		fmt.Fprintf(&buf, "\n%s\n", o.input.Render())
	case settingsEditorModeAgentModelsSelect:
		o.renderAgentModelEditor(&buf)
	case settingsEditorModeEmbeddingModelSelect:
		o.renderEmbeddingModelEditor(&buf)
	case settingsEditorModeEmbeddingConfirm:
		o.renderEmbeddingConfirm(&buf)
	case settingsEditorModeSaving:
		fmt.Fprintf(&buf, "\n  %s\n", styleGray.Render(o.savingLabel))
	}

	if feedback := o.feedback.render(); feedback != "" {
		buf.WriteString(feedback)
	}

	fmt.Fprintf(&buf, "\n  %s\n", styleGray.Render(o.helpLine()))
	return buf.String()
}

func (o *settingsEditorOverlay) renderSelectBlocks(buf *strings.Builder) {
	fmt.Fprintf(buf, "  %s\n", styleBold.Render("Web Search"))
	fmt.Fprintf(buf, "  %s\n\n", styleGray.Render("Space sets the primary provider. Enter configures the current provider."))

	for i, provider := range webSearchProviders {
		cursor := "  "
		if i == o.selection.pos && o.mode == settingsEditorModeSelect {
			cursor = styleCyan.Render("❯") + " "
		}

		status := styleGray.Render("○")
		if provider == o.settings.WebSearchProvider && o.providerConfigured(provider) {
			status = styleYellow.Render("★")
		} else if o.providerConfigured(provider) {
			status = styleGreen.Render("✓")
		}

		name := webSearchProviderDisplayName(provider)
		if i == o.selection.pos {
			name = styleWhite.Render(name)
		} else if !o.providerConfigured(provider) {
			name = styleGray.Render(name)
		}

		detail := styleGray.Render("not configured")
		switch provider {
		case models.WebSearchProviderTavily:
			if o.settings != nil && o.settings.TavilyAPIKey != "" {
				detail = styleGreen.Render(o.settings.TavilyAPIKey)
			}
		case models.WebSearchProviderBrave:
			if o.settings != nil && o.settings.BraveAPIKey != "" {
				detail = styleGreen.Render(o.settings.BraveAPIKey)
			}
		case models.WebSearchProviderFirecrawl:
			if o.settings != nil && o.settings.FirecrawlAPIKey != "" {
				if o.settings.FirecrawlBaseURL != "" {
					detail = styleGreen.Render(o.settings.FirecrawlAPIKey) + styleGray.Render("  "+o.settings.FirecrawlBaseURL)
				} else {
					detail = styleGreen.Render(o.settings.FirecrawlAPIKey)
				}
			}
		}

		fmt.Fprintf(buf, "  %s%s  %s  %s\n", cursor, status, name, detail)
	}

	fmt.Fprintf(buf, "\n  %s\n", styleBold.Render("Agent Models"))
	fmt.Fprintf(buf, "  %s%s\n", o.blockCursor(settingsRowAgentModels), o.agentModelsSummary())

	fmt.Fprintf(buf, "\n  %s\n", styleBold.Render("Embedding Model"))
	fmt.Fprintf(buf, "  %s%s\n", o.blockCursor(settingsRowEmbedding), o.embeddingModelSummary())
}

func (o *settingsEditorOverlay) renderAgentModelEditor(buf *strings.Builder) {
	fmt.Fprintf(buf, "\n  %s\n", styleBold.Render("Select Agent Models"))
	for i, model := range o.chatModels {
		cursor := "  "
		if i == o.agentSelection.pos {
			cursor = styleCyan.Render("❯") + " "
		}
		selPos := o.agentSelection.selectionPos(i)
		badge := styleGray.Render("○")
		if selPos == 0 {
			badge = styleYellow.Render("★")
		} else if selPos > 0 {
			badge = styleCyan.Render(fmt.Sprintf("%d", selPos+1))
		}
		name := modelDisplayName(model)
		if i == o.agentSelection.pos {
			name = styleWhite.Render(name)
		} else {
			name = styleGray.Render(name)
		}
		fmt.Fprintf(buf, "  %s%s  %s\n", cursor, badge, name)
	}
}

func (o *settingsEditorOverlay) renderEmbeddingModelEditor(buf *strings.Builder) {
	fmt.Fprintf(buf, "\n  %s\n", styleBold.Render("Select Embedding Model"))
	for i, model := range o.embeddingModels {
		cursor := "  "
		if i == o.embeddingSelection.pos {
			cursor = styleCyan.Render("❯") + " "
		}
		status := styleGray.Render("○")
		if o.settings.EmbeddingModelID != nil && model.ID == *o.settings.EmbeddingModelID {
			status = styleGreen.Render("✓")
		}
		name := modelDisplayName(model)
		if i == o.embeddingSelection.pos {
			name = styleWhite.Render(name)
		} else {
			name = styleGray.Render(name)
		}
		fmt.Fprintf(buf, "  %s%s  %s\n", cursor, status, name)
	}
}

func (o *settingsEditorOverlay) renderEmbeddingConfirm(buf *strings.Builder) {
	if o.pendingEmbeddingModel == nil {
		return
	}
	fmt.Fprintf(buf, "\n  %s %s\n", styleYellow.Render("⚠"), styleYellow.Render("Switching the embedding model will re-generate all embedding data."))
	fmt.Fprintf(buf, "  %s\n", styleGray.Render("This may take time and incur API costs."))
	fmt.Fprintf(buf, "  %s %s\n", styleYellow.Render("?"), styleWhite.Render("Confirm switch to "+modelDisplayName(*o.pendingEmbeddingModel)+"?"))
}

func (o *settingsEditorOverlay) blockCursor(row int) string {
	if o.mode == settingsEditorModeSelect && o.selection.pos == row {
		return styleCyan.Render("❯") + " "
	}
	return "  "
}

func (o *settingsEditorOverlay) agentModelsSummary() string {
	if !o.modelDataLoaded {
		return styleGray.Render("loading...")
	}
	if len(o.chatModels) == 0 {
		return styleGray.Render("no configured chat models")
	}
	if len(o.currentAgentModelIDs) == 0 {
		return styleGray.Render("not set")
	}

	labels := make([]string, 0, len(o.currentAgentModelIDs))
	byID := make(map[uuid.UUID]models.ModelDto, len(o.chatModels))
	for _, model := range o.chatModels {
		byID[model.ID] = model
	}
	for _, id := range o.currentAgentModelIDs {
		if model, ok := byID[id]; ok {
			labels = append(labels, modelDisplayName(model))
		}
	}
	if len(labels) == 0 {
		return styleGray.Render("not set")
	}
	if len(labels) == 1 {
		return styleYellow.Render("★ ") + styleWhite.Render(labels[0])
	}
	return styleYellow.Render("★ ") + styleWhite.Render(labels[0]) + styleGray.Render(fmt.Sprintf("  + %d fallback(s)", len(labels)-1))
}

func (o *settingsEditorOverlay) embeddingModelSummary() string {
	if !o.modelDataLoaded {
		return styleGray.Render("loading...")
	}
	if len(o.embeddingModels) == 0 {
		return styleGray.Render("no configured embedding models")
	}
	if o.settings == nil || o.settings.EmbeddingModelID == nil {
		return styleGray.Render("not set")
	}
	for _, model := range o.embeddingModels {
		if model.ID == *o.settings.EmbeddingModelID {
			return styleGreen.Render(modelDisplayName(model))
		}
	}
	return styleGray.Render("not set")
}

func (o *settingsEditorOverlay) helpLine() string {
	switch o.mode {
	case settingsEditorModeSelect:
		switch o.selection.pos {
		case settingsRowAgentModels:
			return "↑/↓ navigate  ·  Enter edit agent models  ·  Esc close"
		case settingsRowEmbedding:
			return "↑/↓ navigate  ·  Enter edit embedding model  ·  Esc close"
		default:
			return "↑/↓ navigate  ·  Space set primary  ·  Enter configure  ·  Esc close"
		}
	case settingsEditorModeAPIKeyInput:
		return "Enter save API key  ·  Esc cancel"
	case settingsEditorModeFirecrawlURLInput:
		return "Enter save Firecrawl config  ·  Esc cancel"
	case settingsEditorModeAgentModelsSelect:
		return "↑/↓ navigate  ·  Space select  ·  Enter save  ·  Esc cancel"
	case settingsEditorModeEmbeddingModelSelect:
		return "↑/↓ navigate  ·  Enter save  ·  Esc cancel"
	case settingsEditorModeEmbeddingConfirm:
		return "Enter confirm switch  ·  Esc cancel"
	case settingsEditorModeSaving:
		return "Saving..."
	default:
		return ""
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
