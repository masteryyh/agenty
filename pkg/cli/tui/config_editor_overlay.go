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

package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/models"
)

type configEditorMode uint8

const (
	configEditorModeSelect configEditorMode = iota
	configEditorModeAPIKeyInput
	configEditorModeFirecrawlURLInput
	configEditorModeAgentModelsSelect
	configEditorModeEmbeddingModelSelect
	configEditorModeEmbeddingConfirm
	configEditorModeSaving
)

var webSearchProviders = []models.WebSearchProvider{
	models.WebSearchProviderTavily,
	models.WebSearchProviderBrave,
	models.WebSearchProviderFirecrawl,
}

const (
	configWebSearchProviderCount = 3
	configRowProviderStart       = 0
	configRowAgentModels         = configWebSearchProviderCount
	configRowEmbedding           = configWebSearchProviderCount + 1
)

type configEditorSaveMsg struct {
	config  *models.SystemConfigDto
	err     error
	message string
}

func webSearchProviderDisplayName(provider models.WebSearchProvider) string {
	switch provider {
	case models.WebSearchProviderTavily:
		return "Tavily"
	case models.WebSearchProviderBrave:
		return "Brave"
	case models.WebSearchProviderFirecrawl:
		return "Firecrawl"
	default:
		return string(provider)
	}
}

type configEditorModelDataMsg struct {
	chatModels           []models.ModelDto
	embeddingModels      []models.ModelDto
	currentAgentModelIDs []uuid.UUID
	err                  error
}

type configEditorAgentModelsSavedMsg struct {
	modelIDs []uuid.UUID
	err      error
}

type configEditorOverlay struct {
	backend               backend.Backend
	config                *models.SystemConfigDto
	mode                  configEditorMode
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

func newConfigEditorOverlay(backend backend.Backend, config *models.SystemConfigDto, responseCh chan overlayResponse) *configEditorOverlay {
	overlay := &configEditorOverlay{
		backend:         backend,
		config:          config,
		mode:            configEditorModeSelect,
		selection:       newSelectionModel(configRowEmbedding),
		responseCh:      responseCh,
		savingLabel:     "Loading model config…",
		modelDataLoaded: false,
	}
	overlay.syncSelection()
	return overlay
}

func (o *configEditorOverlay) init() tea.Cmd {
	return o.loadModelData()
}

func (o *configEditorOverlay) update(msg tea.Msg) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return o.handleKey(msg)
	case configEditorModelDataMsg:
		if msg.err != nil {
			o.feedback = o.feedback.setErr("Failed to load model config: " + msg.err.Error())
			o.modelDataLoaded = false
			o.mode = configEditorModeSelect
			return false, nil
		}
		o.chatModels = msg.chatModels
		o.embeddingModels = msg.embeddingModels
		o.currentAgentModelIDs = msg.currentAgentModelIDs
		o.modelDataLoaded = true
		o.syncModelSelections()
		if o.mode == configEditorModeSaving {
			o.mode = configEditorModeSelect
		}
		return false, nil
	case configEditorAgentModelsSavedMsg:
		if msg.err != nil {
			o.feedback = o.feedback.setErr("Failed to update agent models: " + msg.err.Error())
			o.mode = configEditorModeSelect
			return false, nil
		}
		o.currentAgentModelIDs = append([]uuid.UUID(nil), msg.modelIDs...)
		o.feedback = o.feedback.setOK("Agent models updated successfully")
		o.mode = configEditorModeSelect
		o.syncModelSelections()
		return false, nil
	case configEditorSaveMsg:
		if msg.err != nil {
			o.feedback = o.feedback.setErr("Failed to update config: " + msg.err.Error())
			o.mode = configEditorModeSelect
			return false, nil
		}
		o.config = msg.config
		if msg.message != "" {
			o.feedback = o.feedback.setOK(msg.message)
		} else {
			o.feedback = o.feedback.setOK("Config updated successfully")
		}
		o.mode = configEditorModeSelect
		o.syncSelection()
		o.syncModelSelections()
		return false, nil
	default:
		if o.mode == configEditorModeAPIKeyInput || o.mode == configEditorModeFirecrawlURLInput {
			var cmd tea.Cmd
			o.input, cmd = o.input.Update(msg)
			return false, cmd
		}
	}
	return false, nil
}

func (o *configEditorOverlay) handleKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch o.mode {
	case configEditorModeSelect:
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
			if o.selection.pos < configRowAgentModels {
				return false, o.savePrimaryProvider()
			}
		case tea.KeyRunes:
			if string(msg.Runes) == " " && o.selection.pos < configRowAgentModels {
				return false, o.savePrimaryProvider()
			}
		case tea.KeyEnter:
			switch o.selection.pos {
			case configRowAgentModels:
				if !o.modelDataLoaded {
					o.feedback = o.feedback.setWarn("Model data is still loading")
					return false, nil
				}
				if len(o.chatModels) == 0 {
					o.feedback = o.feedback.setWarn("No chat models from configured providers found")
					return false, nil
				}
				o.mode = configEditorModeAgentModelsSelect
				o.feedback = o.feedback.clear()
				return false, nil
			case configRowEmbedding:
				if !o.modelDataLoaded {
					o.feedback = o.feedback.setWarn("Model data is still loading")
					return false, nil
				}
				if len(o.embeddingModels) == 0 {
					o.feedback = o.feedback.setWarn("No embedding models from configured providers found")
					return false, nil
				}
				o.mode = configEditorModeEmbeddingModelSelect
				o.feedback = o.feedback.clear()
				return false, nil
			default:
				return false, o.beginProviderConfig()
			}
		}
	case configEditorModeAPIKeyInput:
		switch msg.Type {
		case tea.KeyEsc:
			o.mode = configEditorModeSelect
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
				input.model.SetValue(firstNonEmptyString(o.config.FirecrawlBaseURL, "https://api.firecrawl.dev"))
				o.input = input
				o.mode = configEditorModeFirecrawlURLInput
				o.feedback = o.feedback.clear()
				return false, cmd
			}
			return false, o.saveProviderConfig(o.editingProvider, key, "")
		}
	case configEditorModeFirecrawlURLInput:
		switch msg.Type {
		case tea.KeyEsc:
			o.mode = configEditorModeSelect
			o.feedback = o.feedback.clear()
			return false, nil
		case tea.KeyEnter:
			return false, o.saveProviderConfig(models.WebSearchProviderFirecrawl, o.pendingAPIKey, o.input.Value())
		}
	case configEditorModeAgentModelsSelect:
		if selection, ok := o.agentSelection.HandleNavKey(msg); ok {
			o.agentSelection = selection
			return false, nil
		}
		switch msg.Type {
		case tea.KeyEsc:
			o.mode = configEditorModeSelect
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
	case configEditorModeEmbeddingModelSelect:
		if selection, ok := o.embeddingSelection.HandleNavKey(msg); ok {
			o.embeddingSelection = selection
			return false, nil
		}
		switch msg.Type {
		case tea.KeyEsc:
			o.mode = configEditorModeSelect
			o.feedback = o.feedback.clear()
			return false, nil
		case tea.KeyEnter:
			chosen := o.embeddingModels[o.embeddingSelection.pos]
			if o.config.EmbeddingModelID != nil && *o.config.EmbeddingModelID != chosen.ID {
				o.pendingEmbeddingModel = &chosen
				o.mode = configEditorModeEmbeddingConfirm
				return false, nil
			}
			return false, o.saveEmbeddingModel(chosen.ID)
		}
	case configEditorModeEmbeddingConfirm:
		switch msg.Type {
		case tea.KeyEsc:
			o.pendingEmbeddingModel = nil
			o.mode = configEditorModeEmbeddingModelSelect
			return false, nil
		case tea.KeyEnter:
			if o.pendingEmbeddingModel == nil {
				o.mode = configEditorModeEmbeddingModelSelect
				return false, nil
			}
			modelID := o.pendingEmbeddingModel.ID
			o.pendingEmbeddingModel = nil
			return false, o.saveEmbeddingModel(modelID)
		}
	case configEditorModeSaving:
		if msg.Type == tea.KeyEsc || msg.Type == tea.KeyCtrlC {
			return false, nil
		}
	}
	return false, nil
}

func (o *configEditorOverlay) beginProviderConfig() tea.Cmd {
	o.editingProvider = webSearchProviders[o.selection.pos]
	input, cmd := o.input.Reset("API Key", false)
	o.input = input
	o.feedback = o.feedback.clear()
	o.mode = configEditorModeAPIKeyInput
	return cmd
}

func (o *configEditorOverlay) savePrimaryProvider() tea.Cmd {
	provider := webSearchProviders[o.selection.pos]
	if !o.providerConfigured(provider) {
		o.feedback = o.feedback.setWarn("Configure this provider before setting it as primary")
		return nil
	}

	o.mode = configEditorModeSaving
	o.savingLabel = "Saving primary provider…"
	backend := o.backend
	return func() tea.Msg {
		updated, err := backend.UpdateSystemConfig(&models.UpdateSystemConfigDto{
			WebSearchProvider: &provider,
		})
		return configEditorSaveMsg{config: updated, err: err, message: "Primary web search provider updated"}
	}
}

func (o *configEditorOverlay) saveProviderConfig(provider models.WebSearchProvider, apiKey, baseURL string) tea.Cmd {
	dto := &models.UpdateSystemConfigDto{}
	switch provider {
	case models.WebSearchProviderTavily:
		dto.TavilyAPIKey = &apiKey
	case models.WebSearchProviderBrave:
		dto.BraveAPIKey = &apiKey
	case models.WebSearchProviderFirecrawl:
		dto.FirecrawlAPIKey = &apiKey
		dto.FirecrawlBaseURL = &baseURL
	}

	o.mode = configEditorModeSaving
	o.savingLabel = "Saving provider config…"
	backend := o.backend
	return func() tea.Msg {
		updated, err := backend.UpdateSystemConfig(dto)
		return configEditorSaveMsg{config: updated, err: err, message: "Web search provider config updated"}
	}
}

func (o *configEditorOverlay) saveAgentModels() tea.Cmd {
	modelIDs := make([]uuid.UUID, len(o.agentSelection.selected))
	for i, idx := range o.agentSelection.selected {
		modelIDs[i] = o.chatModels[idx].ID
	}

	o.mode = configEditorModeSaving
	o.savingLabel = "Saving agent models…"
	backend := o.backend
	return func() tea.Msg {
		agents, err := backend.ListAgents(1, 100)
		if err != nil {
			return configEditorAgentModelsSavedMsg{err: err}
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
		return configEditorAgentModelsSavedMsg{modelIDs: modelIDs, err: err}
	}
}

func (o *configEditorOverlay) saveEmbeddingModel(modelID uuid.UUID) tea.Cmd {
	o.mode = configEditorModeSaving
	o.savingLabel = "Saving embedding model…"
	backend := o.backend
	return func() tea.Msg {
		updated, err := backend.UpdateSystemConfig(&models.UpdateSystemConfigDto{EmbeddingModelID: &modelID})
		return configEditorSaveMsg{config: updated, err: err, message: "Active embedding model updated"}
	}
}

func (o *configEditorOverlay) loadModelData() tea.Cmd {
	backend := o.backend
	return func() tea.Msg {
		modelResult, err := backend.ListModels(1, 500)
		if err != nil {
			return configEditorModelDataMsg{err: fmt.Errorf("failed to list models: %w", err)}
		}
		agentResult, err := backend.ListAgents(1, 100)
		if err != nil {
			return configEditorModelDataMsg{err: fmt.Errorf("failed to list agents: %w", err)}
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

		return configEditorModelDataMsg{
			chatModels:           chatModels,
			embeddingModels:      embeddingModels,
			currentAgentModelIDs: currentAgentModelIDs,
		}
	}
}

func (o *configEditorOverlay) providerConfigured(provider models.WebSearchProvider) bool {
	if o.config == nil {
		return false
	}
	for _, configured := range o.config.ConfiguredWebSearchProviders {
		if configured == provider {
			return true
		}
	}
	return false
}

func (o *configEditorOverlay) syncSelection() {
	if o.config == nil {
		return
	}
	cursor := 0
	for i, provider := range webSearchProviders {
		if provider == o.config.WebSearchProvider {
			cursor = i
			break
		}
	}
	o.selection = o.selection.withCursor(cursor)
}

func (o *configEditorOverlay) syncModelSelections() {
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
	if o.config != nil && o.config.EmbeddingModelID != nil {
		for i, model := range o.embeddingModels {
			if model.ID == *o.config.EmbeddingModelID {
				embedCursor = i
				break
			}
		}
	}
	o.embeddingSelection = newSelectionModel(max(0, len(o.embeddingModels)-1)).withCursor(embedCursor)
}

func (o *configEditorOverlay) render(width, _ int) string {
	var buf strings.Builder
	sep := max(min(56, width-4), 10)
	fmt.Fprintf(&buf, "\n  %s\n", styleBold.Render("System Config"))
	fmt.Fprintf(&buf, "  %s\n\n", styleGray.Render(strings.Repeat("─", sep)))

	o.renderSelectBlocks(&buf)

	switch o.mode {
	case configEditorModeAPIKeyInput, configEditorModeFirecrawlURLInput:
		fmt.Fprintf(&buf, "\n%s\n", o.input.Render())
	case configEditorModeAgentModelsSelect:
		o.renderAgentModelEditor(&buf)
	case configEditorModeEmbeddingModelSelect:
		o.renderEmbeddingModelEditor(&buf)
	case configEditorModeEmbeddingConfirm:
		o.renderEmbeddingConfirm(&buf)
	case configEditorModeSaving:
		fmt.Fprintf(&buf, "\n  %s\n", styleGray.Render(o.savingLabel))
	}

	if feedback := o.feedback.render(); feedback != "" {
		buf.WriteString(feedback)
	}

	fmt.Fprintf(&buf, "\n  %s\n", styleGray.Render(o.helpLine()))
	return buf.String()
}

func (o *configEditorOverlay) renderSelectBlocks(buf *strings.Builder) {
	fmt.Fprintf(buf, "  %s\n", styleBold.Render("Web Search"))
	fmt.Fprintf(buf, "  %s\n\n", styleGray.Render("Space sets the primary provider. Enter configures the current provider."))

	for i, provider := range webSearchProviders {
		cursor := "  "
		if i == o.selection.pos && o.mode == configEditorModeSelect {
			cursor = styleCyan.Render("❯") + " "
		}

		status := styleGray.Render("○")
		if provider == o.config.WebSearchProvider && o.providerConfigured(provider) {
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
			if o.config != nil && o.config.TavilyAPIKey != "" {
				detail = styleGreen.Render(o.config.TavilyAPIKey)
			}
		case models.WebSearchProviderBrave:
			if o.config != nil && o.config.BraveAPIKey != "" {
				detail = styleGreen.Render(o.config.BraveAPIKey)
			}
		case models.WebSearchProviderFirecrawl:
			if o.config != nil && o.config.FirecrawlAPIKey != "" {
				if o.config.FirecrawlBaseURL != "" {
					detail = styleGreen.Render(o.config.FirecrawlAPIKey) + styleGray.Render("  "+o.config.FirecrawlBaseURL)
				} else {
					detail = styleGreen.Render(o.config.FirecrawlAPIKey)
				}
			}
		}

		fmt.Fprintf(buf, "  %s%s  %s  %s\n", cursor, status, name, detail)
	}

	fmt.Fprintf(buf, "\n  %s\n", styleBold.Render("Agent Models"))
	fmt.Fprintf(buf, "  %s%s\n", o.blockCursor(configRowAgentModels), o.agentModelsSummary())

	fmt.Fprintf(buf, "\n  %s\n", styleBold.Render("Embedding Model"))
	fmt.Fprintf(buf, "  %s%s\n", o.blockCursor(configRowEmbedding), o.embeddingModelSummary())
}

func (o *configEditorOverlay) renderAgentModelEditor(buf *strings.Builder) {
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

func (o *configEditorOverlay) renderEmbeddingModelEditor(buf *strings.Builder) {
	fmt.Fprintf(buf, "\n  %s\n", styleBold.Render("Select Embedding Model"))
	for i, model := range o.embeddingModels {
		cursor := "  "
		if i == o.embeddingSelection.pos {
			cursor = styleCyan.Render("❯") + " "
		}
		status := styleGray.Render("○")
		if o.config.EmbeddingModelID != nil && model.ID == *o.config.EmbeddingModelID {
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

func (o *configEditorOverlay) renderEmbeddingConfirm(buf *strings.Builder) {
	if o.pendingEmbeddingModel == nil {
		return
	}
	fmt.Fprintf(buf, "\n  %s %s\n", styleYellow.Render("⚠"), styleYellow.Render("Switching the embedding model will re-generate all embedding data."))
	fmt.Fprintf(buf, "  %s\n", styleGray.Render("This may take time and incur API costs."))
	fmt.Fprintf(buf, "  %s %s\n", styleYellow.Render("?"), styleWhite.Render("Confirm switch to "+modelDisplayName(*o.pendingEmbeddingModel)+"?"))
}

func (o *configEditorOverlay) blockCursor(row int) string {
	if o.mode == configEditorModeSelect && o.selection.pos == row {
		return styleCyan.Render("❯") + " "
	}
	return "  "
}

func (o *configEditorOverlay) agentModelsSummary() string {
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

func (o *configEditorOverlay) embeddingModelSummary() string {
	if !o.modelDataLoaded {
		return styleGray.Render("loading...")
	}
	if len(o.embeddingModels) == 0 {
		return styleGray.Render("no configured embedding models")
	}
	if o.config == nil || o.config.EmbeddingModelID == nil {
		return styleGray.Render("not set")
	}
	for _, model := range o.embeddingModels {
		if model.ID == *o.config.EmbeddingModelID {
			return styleGreen.Render(modelDisplayName(model))
		}
	}
	return styleGray.Render("not set")
}

func (o *configEditorOverlay) helpLine() string {
	switch o.mode {
	case configEditorModeSelect:
		switch o.selection.pos {
		case configRowAgentModels:
			return "↑/↓ navigate  ·  Enter edit agent models  ·  Esc close"
		case configRowEmbedding:
			return "↑/↓ navigate  ·  Enter edit embedding model  ·  Esc close"
		default:
			return "↑/↓ navigate  ·  Space set primary  ·  Enter configure  ·  Esc close"
		}
	case configEditorModeAPIKeyInput:
		return "Enter save API key  ·  Esc cancel"
	case configEditorModeFirecrawlURLInput:
		return "Enter save Firecrawl config  ·  Esc cancel"
	case configEditorModeAgentModelsSelect:
		return "↑/↓ navigate  ·  Space select  ·  Enter save  ·  Esc cancel"
	case configEditorModeEmbeddingModelSelect:
		return "↑/↓ navigate  ·  Enter save  ·  Esc cancel"
	case configEditorModeEmbeddingConfirm:
		return "Enter confirm switch  ·  Esc cancel"
	case configEditorModeSaving:
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
