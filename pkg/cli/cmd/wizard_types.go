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
	"github.com/masteryyh/agenty/pkg/models"
)

type wizStep int

const (
	wizStepWelcome wizStep = iota
	wizStepProviderList
	wizStepProviderInput
	wizStepWebSearchSelect
	wizStepWebSearchKey
	wizStepFirecrawlURL
	wizStepModelSelect
	wizStepEmbedSelect
	wizStepSaving
	wizStepDone
)

const wizMaxModels = 4

type wizSaveProviderMsg struct{ err error }
type wizSaveWebSearchMsg struct{ err error }
type wizAssignAgentModelsMsg struct{ err error }
type wizSaveEmbedModelMsg struct{ err error }
type wizModelsLoadedMsg struct {
	models []models.ModelDto
	err    error
}
type wizDoneTimerMsg struct{}

type wizWSEntry struct {
	label    string
	provider models.WebSearchProvider
}

var wizWSProviders = []wizWSEntry{
	{"Tavily", models.WebSearchProviderTavily},
	{"Brave", models.WebSearchProviderBrave},
	{"Firecrawl", models.WebSearchProviderFirecrawl},
}

// wizNav is a value-type navigation cursor for list steps.
// pos is the current position; max is the inclusive upper bound.
type wizNav struct {
	pos int
	max int
}

func newWizNav(max int) wizNav { return wizNav{max: max} }

func (n wizNav) Up() wizNav {
	if n.pos > 0 {
		n.pos--
	}
	return n
}

func (n wizNav) Down() wizNav {
	if n.pos < n.max {
		n.pos++
	}
	return n
}

// HandleNavKey moves the cursor on ↑/↓/k/j. Returns (updated nav, consumed).
func (n wizNav) HandleNavKey(msg tea.KeyMsg) (wizNav, bool) {
	switch msg.Type {
	case tea.KeyUp:
		return n.Up(), true
	case tea.KeyDown:
		return n.Down(), true
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "k":
			return n.Up(), true
		case "j":
			return n.Down(), true
		}
	}
	return n, false
}
