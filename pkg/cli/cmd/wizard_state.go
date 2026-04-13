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
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/cli/theme"
	"github.com/masteryyh/agenty/pkg/models"
)

type wizInput struct {
	model textinput.Model
	label string
}

func newWizTextInput(label string, masked bool) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.TextStyle = theme.White
	ti.PlaceholderStyle = theme.Muted
	ti.Cursor.Style = theme.Cyan
	if masked {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '●'
	}
	return ti
}

func (i wizInput) Reset(label string, masked bool) (wizInput, tea.Cmd) {
	ti := newWizTextInput(label, masked)
	cmd := ti.Focus()
	return wizInput{model: ti, label: label}, cmd
}

func (i wizInput) Update(msg tea.Msg) (wizInput, tea.Cmd) {
	var cmd tea.Cmd
	i.model, cmd = i.model.Update(msg)
	return i, cmd
}

func (i wizInput) Value() string {
	return strings.TrimSpace(i.model.Value())
}

func (i wizInput) Render() string {
	return "  " + styleCyan.Render("❯") + " " + styleWhite.Render(i.label+":") + "  " + i.model.View()
}

type feedbackKind uint8

const (
	feedbackNone feedbackKind = iota
	feedbackOK
	feedbackWarn
	feedbackErr
)

// wizFeedback is an immutable message+severity pair shown below list steps.
type wizFeedback struct {
	msg  string
	kind feedbackKind
}

func (f wizFeedback) setOK(msg string) wizFeedback   { return wizFeedback{msg: msg, kind: feedbackOK} }
func (f wizFeedback) setWarn(msg string) wizFeedback { return wizFeedback{msg: msg, kind: feedbackWarn} }
func (f wizFeedback) setErr(msg string) wizFeedback  { return wizFeedback{msg: msg, kind: feedbackErr} }
func (wizFeedback) clear() wizFeedback               { return wizFeedback{} }

func (f wizFeedback) render() string {
	if f.msg == "" {
		return ""
	}
	switch f.kind {
	case feedbackOK:
		return "\n  " + styleGreen.Render(f.msg) + "\n"
	case feedbackErr:
		return "\n  " + styleRed.Render(f.msg) + "\n"
	case feedbackWarn:
		return "\n  " + styleYellow.Render("⚠") + "  " + f.msg + "\n"
	}
	return "\n  " + f.msg + "\n"
}

// wizListItem describes a single row in a selectable-list panel.
// checked=true shows a green ✓; false shows a gray ○.
// detail may be pre-styled with ANSI codes.
type wizListItem struct {
	checked bool
	name    string
	detail  string
}

// wizardModel is the BubbleTea model driving the setup wizard.
type wizardModel struct {
	backend       backend.Backend
	width, height int
	step          wizStep

	// Provider configuration step
	providers        []models.ModelProviderDto
	configuredIDs    map[string]bool
	provNav          wizNav
	selectedProvIdx  int
	lastSavedKeyMask string

	// Web search configuration step
	wsNav             wizNav
	wsKey             string
	wsCurrentProvider models.WebSearchProvider
	wsConfiguredKeys  map[string]string

	// Chat model selection step
	chatModels    []models.ModelDto
	chatLabels    []string
	chatNav       wizNav
	selectedOrder []int // [0]=primary, [1+]=fallbacks

	// Embedding model selection step
	embedModels         []models.ModelDto
	embedLabels         []string
	embedNav            wizNav
	currentEmbedModelID *uuid.UUID

	input    wizInput
	feedback wizFeedback

	savingLabel string
	done        bool
	aborted     bool
}

func newWizardModel(b backend.Backend, providers []models.ModelProviderDto, settings *models.SystemSettingsDto) wizardModel {
	m := wizardModel{
		backend:          b,
		providers:        providers,
		configuredIDs:    make(map[string]bool),
		wsConfiguredKeys: make(map[string]string),
		provNav:          newWizNav(len(providers)),
		wsNav:            newWizNav(len(wizWSProviders)),
	}
	for _, p := range providers {
		if p.APIKeyCensored != "<not set>" {
			m.configuredIDs[p.ID.String()] = true
		}
	}
	if settings != nil {
		m.wsCurrentProvider = settings.WebSearchProvider
		if settings.TavilyAPIKey != "" {
			m.wsConfiguredKeys[string(models.WebSearchProviderTavily)] = settings.TavilyAPIKey
		}
		if settings.BraveAPIKey != "" {
			m.wsConfiguredKeys[string(models.WebSearchProviderBrave)] = settings.BraveAPIKey
		}
		if settings.FirecrawlAPIKey != "" {
			m.wsConfiguredKeys[string(models.WebSearchProviderFirecrawl)] = settings.FirecrawlAPIKey
		}
		for i, ws := range wizWSProviders {
			if ws.provider == settings.WebSearchProvider {
				m.wsNav.pos = i
				break
			}
		}
		m.currentEmbedModelID = settings.EmbeddingModelID
	}
	return m
}

// populateChatModels splits loaded models into chat and embedding groups,
// sorts each by ID, builds display labels, and resets selection cursors.
func (m wizardModel) populateChatModels(all []models.ModelDto) wizardModel {
	var chat, embed []models.ModelDto
	for _, mdl := range all {
		if mdl.Provider == nil || !m.configuredIDs[mdl.Provider.ID.String()] {
			continue
		}
		if mdl.EmbeddingModel {
			embed = append(embed, mdl)
		} else {
			chat = append(chat, mdl)
		}
	}
	sort.Slice(chat, func(i, j int) bool { return chat[i].ID.String() < chat[j].ID.String() })
	sort.Slice(embed, func(i, j int) bool { return embed[i].ID.String() < embed[j].ID.String() })

	m.chatModels = chat
	m.chatLabels = make([]string, len(chat))
	for i, mdl := range chat {
		m.chatLabels[i] = wizModelLabel(mdl)
	}
	m.chatNav = newWizNav(max(0, len(chat)-1))
	m.selectedOrder = nil

	m.embedModels = embed
	m.embedLabels = make([]string, len(embed))
	embedCursor := 0
	for i, mdl := range embed {
		m.embedLabels[i] = wizModelLabel(mdl)
		if m.currentEmbedModelID != nil && mdl.ID == *m.currentEmbedModelID {
			embedCursor = i
		}
	}
	m.embedNav = wizNav{pos: embedCursor, max: max(0, len(embed)-1)}
	return m
}

// wizModelLabel returns a "Provider/Name" display label for a model.
func wizModelLabel(mdl models.ModelDto) string {
	if mdl.Provider != nil {
		return mdl.Provider.Name + "/" + mdl.Name
	}
	return mdl.Name
}

// toggleModelSelection adds or removes idx from the ordered selection,
// enforcing the wizMaxModels limit.
func (m wizardModel) toggleModelSelection(idx int) wizardModel {
	for i, sel := range m.selectedOrder {
		if sel == idx {
			m.selectedOrder = append(m.selectedOrder[:i], m.selectedOrder[i+1:]...)
			m.feedback = m.feedback.clear()
			return m
		}
	}
	if len(m.selectedOrder) >= wizMaxModels {
		m.feedback = m.feedback.setWarn(fmt.Sprintf(
			"Maximum %d models allowed (1 primary + %d fallbacks)", wizMaxModels, wizMaxModels-1,
		))
		return m
	}
	m.selectedOrder = append(m.selectedOrder, idx)
	m.feedback = m.feedback.clear()
	return m
}

// modelSelectionPos returns the selection rank of idx (0=primary), or -1 if unselected.
func (m wizardModel) modelSelectionPos(idx int) int {
	for i, sel := range m.selectedOrder {
		if sel == idx {
			return i
		}
	}
	return -1
}

// startLoadingModels transitions to the saving/spinner state and fires ListModels.
func (m wizardModel) startLoadingModels() (wizardModel, tea.Cmd) {
	b := m.backend
	m.step = wizStepSaving
	m.savingLabel = "Loading models…"
	m.feedback = m.feedback.clear()
	return m, func() tea.Msg {
		result, err := b.ListModels(1, 500)
		if err != nil {
			return wizModelsLoadedMsg{err: err}
		}
		return wizModelsLoadedMsg{models: result.Data}
	}
}

// finishWithSuccess marks the wizard complete, calls SetInitialized, and starts the done timer.
func (m wizardModel) finishWithSuccess() (wizardModel, tea.Cmd) {
	_ = m.backend.SetInitialized()
	m.done = true
	m.step = wizStepDone
	return m, tea.Tick(1500*time.Millisecond, func(_ time.Time) tea.Msg {
		return wizDoneTimerMsg{}
	})
}
