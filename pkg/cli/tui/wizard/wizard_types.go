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

package wizard

import "github.com/masteryyh/agenty/pkg/models"

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

type wizNav = selectionModel

func newWizNav(max int) wizNav { return newSelectionModel(max) }
