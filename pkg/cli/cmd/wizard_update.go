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
	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/models"
)

func (m wizardModel) Init() tea.Cmd { return nil }

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case wizModelsLoadedMsg:
		return m.onModelsLoaded(msg)
	case wizSaveProviderMsg:
		return m.onSaveProvider(msg)
	case wizSaveWebSearchMsg:
		return m.onSaveWebSearch(msg)
	case wizAssignAgentModelsMsg:
		return m.onAssignAgentModels(msg)
	case wizSaveEmbedModelMsg:
		return m.onSaveEmbedModel(msg)
	case wizDoneTimerMsg:
		return m, tea.Quit
	case tea.KeyMsg:
		return m.handleKey(msg)
	default:
		if m.step == wizStepProviderInput || m.step == wizStepWebSearchKey || m.step == wizStepFirecrawlURL {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

// --- Message handlers ---

func (m wizardModel) onModelsLoaded(msg wizModelsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.feedback = m.feedback.setErr("Failed to load models: " + msg.err.Error())
		m.step = wizStepProviderList
		return m, nil
	}
	m = m.populateChatModels(msg.models)
	if len(m.chatModels) == 0 {
		m.feedback = m.feedback.setWarn("No models found for configured providers. Please configure more providers.")
		m.step = wizStepProviderList
		return m, nil
	}
	m.step = wizStepWebSearchSelect
	m.feedback = m.feedback.clear()
	return m, nil
}

func (m wizardModel) onSaveProvider(msg wizSaveProviderMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.feedback = m.feedback.setErr("Failed to save API key: " + msg.err.Error())
	} else {
		pid := m.providers[m.selectedProvIdx].ID.String()
		m.configuredIDs[pid] = true
		m.providers[m.selectedProvIdx].APIKeyCensored = m.lastSavedKeyMask
		m.feedback = m.feedback.setOK("✓ API key saved for " + m.providers[m.selectedProvIdx].Name)
	}
	m.step = wizStepProviderList
	return m, nil
}

func (m wizardModel) onSaveWebSearch(msg wizSaveWebSearchMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.feedback = m.feedback.setErr("Failed to save web search config: " + msg.err.Error())
		m.step = wizStepWebSearchSelect
		return m, nil
	}
	m.wsCurrentProvider = wizWSProviders[m.wsNav.pos].provider
	if m.lastSavedKeyMask != "" {
		m.wsConfiguredKeys[string(m.wsCurrentProvider)] = m.lastSavedKeyMask
	}
	m.feedback = m.feedback.clear()
	m.step = wizStepModelSelect
	return m, nil
}

func (m wizardModel) onAssignAgentModels(msg wizAssignAgentModelsMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.feedback = m.feedback.setErr("Failed to save agent models: " + msg.err.Error())
		m.step = wizStepModelSelect
		return m, nil
	}
	if len(m.embedModels) > 0 {
		m.step = wizStepEmbedSelect
		m.feedback = m.feedback.clear()
		return m, nil
	}
	return m.finishWithSuccess()
}

func (m wizardModel) onSaveEmbedModel(msg wizSaveEmbedModelMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.feedback = m.feedback.setErr("Failed to save embedding model: " + msg.err.Error())
		m.step = wizStepEmbedSelect
		return m, nil
	}
	return m.finishWithSuccess()
}

// --- Key dispatch ---

func (m wizardModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.step {
	case wizStepWelcome:
		return m.handleWelcomeKey(msg)
	case wizStepProviderList:
		return m.handleProviderListKey(msg)
	case wizStepProviderInput:
		return m.handleProviderInputKey(msg)
	case wizStepWebSearchSelect:
		return m.handleWebSearchSelectKey(msg)
	case wizStepWebSearchKey:
		return m.handleWebSearchKeyInput(msg)
	case wizStepFirecrawlURL:
		return m.handleFirecrawlURLInput(msg)
	case wizStepModelSelect:
		return m.handleModelSelectKey(msg)
	case wizStepEmbedSelect:
		return m.handleEmbedSelectKey(msg)
	case wizStepDone:
		return m, tea.Quit
	}
	return m, nil
}

// --- Per-step key handlers ---

func (m wizardModel) handleWelcomeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "y", "Y":
			m.step = wizStepProviderList
			m.feedback = m.feedback.clear()
		case "n", "N":
			m.aborted = true
			return m, tea.Quit
		}
	case tea.KeyEsc:
		m.aborted = true
		return m, tea.Quit
	}
	return m, nil
}

func (m wizardModel) handleProviderListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if nav, ok := m.provNav.HandleNavKey(msg); ok {
		m.provNav = nav
		return m, nil
	}
	switch msg.Type {
	case tea.KeyEnter:
		if m.provNav.pos == m.provNav.max {
			return m.startLoadingModels()
		}
		m.selectedProvIdx = m.provNav.pos
		var cmd tea.Cmd
		m.input, cmd = m.input.Reset("API Key", false)
		m.feedback = m.feedback.clear()
		m.step = wizStepProviderInput
		return m, cmd
	case tea.KeyEsc:
		if len(m.configuredIDs) > 0 {
			return m.startLoadingModels()
		}
		m.feedback = m.feedback.setWarn("Please configure at least one provider to continue")
	}
	return m, nil
}

func (m wizardModel) handleProviderInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.step = wizStepProviderList
		m.feedback = m.feedback.clear()
		return m, nil
	case tea.KeyEnter:
		key := m.input.Value()
		if key == "" {
			return m, nil
		}
		m.lastSavedKeyMask = maskWizKey(key)
		prov := m.providers[m.selectedProvIdx]
		b := m.backend
		m.step = wizStepSaving
		m.savingLabel = "Saving API key…"
		return m, func() tea.Msg {
			_, err := b.UpdateProvider(prov.ID, &models.UpdateModelProviderDto{APIKey: key})
			return wizSaveProviderMsg{err: err}
		}
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

func (m wizardModel) handleWebSearchSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if nav, ok := m.wsNav.HandleNavKey(msg); ok {
		m.wsNav = nav
		return m, nil
	}
	switch msg.Type {
	case tea.KeyEnter:
		if m.wsNav.pos == m.wsNav.max {
			m.feedback = m.feedback.clear()
			m.step = wizStepModelSelect
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Reset("API Key", false)
		m.feedback = m.feedback.clear()
		m.step = wizStepWebSearchKey
		return m, cmd
	case tea.KeyEsc:
		m.feedback = m.feedback.clear()
		m.step = wizStepModelSelect
	}
	return m, nil
}

func (m wizardModel) handleWebSearchKeyInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.step = wizStepWebSearchSelect
		m.feedback = m.feedback.clear()
		return m, nil
	case tea.KeyEnter:
		key := m.input.Value()
		if key == "" {
			return m, nil
		}
		m.wsKey = key
		m.lastSavedKeyMask = maskWizKey(key)
		if wizWSProviders[m.wsNav.pos].provider == models.WebSearchProviderFirecrawl {
			var cmd tea.Cmd
			m.input, cmd = m.input.Reset("Base URL", false)
			m.feedback = m.feedback.clear()
			m.step = wizStepFirecrawlURL
			return m, cmd
		}
		return m.saveWebSearchProvider(key)
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

func (m wizardModel) handleFirecrawlURLInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.feedback = m.feedback.clear()
		m.step = wizStepModelSelect
		return m, nil
	case tea.KeyEnter:
		url := m.input.Value()
		wsKey := m.wsKey
		b := m.backend
		provider := models.WebSearchProviderFirecrawl
		dto := &models.UpdateSystemSettingsDto{
			WebSearchProvider: &provider,
			FirecrawlAPIKey:   &wsKey,
		}
		if url != "" {
			dto.FirecrawlBaseURL = &url
		}
		m.step = wizStepSaving
		m.savingLabel = "Saving Firecrawl config…"
		return m, func() tea.Msg {
			_, err := b.UpdateSystemSettings(dto)
			return wizSaveWebSearchMsg{err: err}
		}
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

func (m wizardModel) handleModelSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if nav, ok := m.chatNav.HandleNavKey(msg); ok {
		m.chatNav = nav
		return m, nil
	}
	switch msg.Type {
	case tea.KeySpace:
		m = m.toggleModelSelection(m.chatNav.pos)
	case tea.KeyRunes:
		if string(msg.Runes) == " " {
			m = m.toggleModelSelection(m.chatNav.pos)
		}
	case tea.KeyEnter:
		if len(m.selectedOrder) == 0 {
			m.feedback = m.feedback.setWarn("Select at least one model to continue")
			return m, nil
		}
		return m.saveAgentModels()
	}
	return m, nil
}

func (m wizardModel) handleEmbedSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if nav, ok := m.embedNav.HandleNavKey(msg); ok {
		m.embedNav = nav
		return m, nil
	}
	switch msg.Type {
	case tea.KeyEnter:
		embedID := m.embedModels[m.embedNav.pos].ID
		b := m.backend
		m.step = wizStepSaving
		m.savingLabel = "Saving embedding model…"
		return m, func() tea.Msg {
			_, err := b.UpdateSystemSettings(&models.UpdateSystemSettingsDto{EmbeddingModelID: &embedID})
			return wizSaveEmbedModelMsg{err: err}
		}
	case tea.KeyEsc:
		return m.finishWithSuccess()
	}
	return m, nil
}

// --- Shared action helpers ---

// saveWebSearchProvider saves Tavily or Brave web search settings.
func (m wizardModel) saveWebSearchProvider(key string) (tea.Model, tea.Cmd) {
	provider := wizWSProviders[m.wsNav.pos].provider
	b := m.backend
	dto := &models.UpdateSystemSettingsDto{WebSearchProvider: &provider}
	switch provider {
	case models.WebSearchProviderTavily:
		dto.TavilyAPIKey = &key
	case models.WebSearchProviderBrave:
		dto.BraveAPIKey = &key
	}
	m.step = wizStepSaving
	m.savingLabel = "Saving web search config…"
	return m, func() tea.Msg {
		_, err := b.UpdateSystemSettings(dto)
		return wizSaveWebSearchMsg{err: err}
	}
}

// saveAgentModels upserts the default agent with the selected chat model IDs.
func (m wizardModel) saveAgentModels() (tea.Model, tea.Cmd) {
	modelIDs := make([]uuid.UUID, len(m.selectedOrder))
	for i, idx := range m.selectedOrder {
		modelIDs[i] = m.chatModels[idx].ID
	}
	b := m.backend
	m.step = wizStepSaving
	m.savingLabel = "Saving agent models…"
	return m, func() tea.Msg {
		agents, err := b.ListAgents(1, 100)
		if err != nil {
			return wizAssignAgentModelsMsg{err: err}
		}
		if agent := findDefaultAgent(agents.Data); agent != nil {
			err = b.UpdateAgent(agent.ID, &models.UpdateAgentDto{ModelIDs: &modelIDs})
		} else {
			soul := ""
			_, err = b.CreateAgent(&models.CreateAgentDto{
				Name:      "default",
				Soul:      &soul,
				IsDefault: true,
				ModelIDs:  modelIDs,
			})
		}
		return wizAssignAgentModelsMsg{err: err}
	}
}

// findDefaultAgent returns the first agent with IsDefault=true, then by name "default", or nil.
func findDefaultAgent(agents []models.AgentDto) *models.AgentDto {
	for i := range agents {
		if agents[i].IsDefault {
			return &agents[i]
		}
	}
	for i := range agents {
		if agents[i].Name == "default" {
			return &agents[i]
		}
	}
	return nil
}
