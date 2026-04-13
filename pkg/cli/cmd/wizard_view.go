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

	"github.com/masteryyh/agenty/pkg/models"
)

func (m wizardModel) View() string {
	switch m.step {
	case wizStepWelcome:
		return m.viewWelcome()
	case wizStepProviderList:
		return m.viewProviderList()
	case wizStepProviderInput:
		return m.viewProviderInput()
	case wizStepWebSearchSelect:
		return m.viewWebSearchSelect()
	case wizStepWebSearchKey:
		return m.viewWebSearchKey()
	case wizStepFirecrawlURL:
		return m.viewFirecrawlURL()
	case wizStepModelSelect:
		return m.viewModelSelect()
	case wizStepEmbedSelect:
		return m.viewEmbedSelect()
	case wizStepSaving:
		return m.viewSaving()
	case wizStepDone:
		return m.viewDone()
	}
	return ""
}

func (m wizardModel) viewWelcome() string {
	return m.renderHeader("First-Time Setup") +
		"  Welcome to Agenty!\n\n" +
		"  Let's configure the API keys for the providers you want\n" +
		"  to use, and choose your models.\n\n" +
		"  Press " + styleWhite.Render("Esc") + " at any time to skip a step.\n\n" +
		"  " + styleYellow.Render("?") + " Start setup now? " + styleGray.Render("[y/N]") + ": "
}

func (m wizardModel) viewProviderList() string {
	items := make([]wizListItem, len(m.providers))
	for i, p := range m.providers {
		var detail string
		if m.configuredIDs[p.ID.String()] && p.APIKeyCensored != "<not set>" {
			detail = styleGreen.Render(p.APIKeyCensored)
		} else {
			detail = styleGray.Render("not configured")
		}
		items[i] = wizListItem{checked: m.configuredIDs[p.ID.String()], name: p.Name, detail: detail}
	}
	return m.renderSelectableList(
		"Configure Provider API Keys",
		"↑/↓ navigate  ·  Enter configure/continue  ·  Esc skip",
		items, m.provNav, true,
	)
}

func (m wizardModel) viewProviderInput() string {
	title := m.providers[m.selectedProvIdx].Name + "  API Key"
	return m.renderHeader(title) +
		m.input.Render() + "\n\n" +
		"  " + styleGray.Render("Enter confirm  ·  Esc cancel") + "\n" +
		m.feedback.render()
}

func (m wizardModel) viewWebSearchSelect() string {
	items := make([]wizListItem, len(wizWSProviders))
	for i, ws := range wizWSProviders {
		isActive := m.wsCurrentProvider == ws.provider && m.wsCurrentProvider != models.WebSearchProviderDisabled
		var detail string
		if key, ok := m.wsConfiguredKeys[string(ws.provider)]; ok && key != "" {
			if isActive {
				detail = styleGreen.Render(key)
			} else {
				detail = styleGray.Render(key)
			}
		} else {
			detail = styleGray.Render("not configured")
		}
		items[i] = wizListItem{checked: isActive, name: ws.label, detail: detail}
	}
	return m.renderSelectableList(
		"Configure Web Search  "+styleGray.Render("(optional)"),
		"↑/↓ navigate  ·  Enter configure/continue  ·  Esc skip",
		items, m.wsNav, true,
	)
}

func (m wizardModel) viewWebSearchKey() string {
	title := wizWSProviders[m.wsNav.pos].label + "  API Key"
	return m.renderHeader(title) +
		m.input.Render() + "\n\n" +
		"  " + styleGray.Render("Enter confirm  ·  Esc cancel") + "\n" +
		m.feedback.render()
}

func (m wizardModel) viewFirecrawlURL() string {
	return m.renderHeader("Firecrawl Base URL  "+styleGray.Render("(optional)")) +
		m.input.Render() + "\n\n" +
		"  " + styleGray.Render("Enter confirm  ·  Esc skip") + "\n" +
		m.feedback.render()
}

func (m wizardModel) viewModelSelect() string {
	var buf strings.Builder
	buf.WriteString(m.renderHeader("Select Models  " + styleGray.Render(
		fmt.Sprintf("(1 primary + up to %d fallbacks)", wizMaxModels-1),
	)))
	for i, label := range m.chatLabels {
		selPos := m.modelSelectionPos(i)
		cursor := "  "
		if i == m.chatNav.pos {
			cursor = styleCyan.Render("❯") + " "
		}
		var badge, displayLabel string
		switch {
		case selPos == 0:
			badge = styleYellow.Render("★") + " "
			displayLabel = styleWhite.Render(label)
		case selPos > 0:
			badge = styleCyan.Render(fmt.Sprintf("%d", selPos+1)) + " "
			displayLabel = styleWhite.Render(label)
		default:
			badge = styleGray.Render("○") + " "
			if i == m.chatNav.pos {
				displayLabel = styleWhite.Render(label)
			} else {
				displayLabel = styleGray.Render(label)
			}
		}
		buf.WriteString("  " + cursor + badge + " " + displayLabel + "\n")
	}
	buf.WriteString("\n  " + styleGray.Render("↑/↓ navigate  ·  Space select  ·  Enter confirm") + "\n")
	switch n := len(m.selectedOrder); {
	case n == 0:
		buf.WriteString("  " + styleYellow.Render("⚠") + "  " + styleGray.Render("Select at least one model") + "\n")
	case n == 1:
		buf.WriteString("  " + styleGreen.Render("✓") + "  " + styleYellow.Render("★ ") +
			styleWhite.Render(m.chatLabels[m.selectedOrder[0]]) + "  " + styleGray.Render("(primary)") + "\n")
	default:
		buf.WriteString("  " + styleGreen.Render("✓") + "  " + styleYellow.Render("★ ") +
			styleWhite.Render(m.chatLabels[m.selectedOrder[0]]) +
			"  " + styleGray.Render(fmt.Sprintf("+ %d fallback(s)", n-1)) + "\n")
	}
	buf.WriteString(m.feedback.render())
	return buf.String()
}

func (m wizardModel) viewEmbedSelect() string {
	var buf strings.Builder
	buf.WriteString(m.renderHeader("Select Embedding Model  " + styleGray.Render("(for knowledge base)")))
	for i, label := range m.embedLabels {
		isCurrent := m.currentEmbedModelID != nil && m.embedModels[i].ID == *m.currentEmbedModelID
		statusIcon := styleGray.Render("○")
		if isCurrent {
			statusIcon = styleGreen.Render("✓")
		}
		cursor := "  "
		if i == m.embedNav.pos {
			cursor = styleCyan.Render("❯") + " "
		}
		displayLabel := styleGray.Render(label)
		if i == m.embedNav.pos {
			displayLabel = styleWhite.Render(label)
		}
		buf.WriteString("  " + cursor + statusIcon + "  " + displayLabel + "\n")
	}
	buf.WriteString("\n")
	buf.WriteString("  " + styleYellow.Render("⚠") + "  " + styleYellow.Render("Changing the embedding model later requires") + "\n")
	buf.WriteString("     " + styleYellow.Render("complete re-vectorization of all knowledge base data.") + "\n")
	buf.WriteString("\n  " + styleGray.Render("↑/↓ navigate  ·  Enter select  ·  Esc skip") + "\n")
	buf.WriteString(m.feedback.render())
	return buf.String()
}

func (m wizardModel) viewSaving() string {
	return "\n  " + styleGray.Render(m.savingLabel) + "\n"
}

func (m wizardModel) viewDone() string {
	return "\n  " + styleGreen.Render("✓") + " " + styleBold.Render("Setup complete!") + "\n\n" +
		"  " + styleGray.Render("Starting Agenty Chat...") + "\n"
}

// renderHeader renders the title and horizontal rule at the top of each wizard step.
func (m wizardModel) renderHeader(title string) string {
	w := m.width - 4
	if w < 20 {
		w = 60
	}
	return "\n  " + styleBold.Render(title) + "\n  " + styleGray.Render(strings.Repeat("─", w)) + "\n\n"
}

// renderSelectableList renders a navigable list of items with an optional "Continue →" row.
// When withContinue is true, nav.max == len(items) and selecting that position means Continue.
func (m wizardModel) renderSelectableList(title, hints string, items []wizListItem, nav wizNav, withContinue bool) string {
	var buf strings.Builder
	buf.WriteString(m.renderHeader(title))

	nameWidth := 10
	for _, it := range items {
		if len(it.name) > nameWidth {
			nameWidth = len(it.name)
		}
	}
	nameWidth += 2

	for i, it := range items {
		statusIcon := styleGray.Render("○")
		if it.checked {
			statusIcon = styleGreen.Render("✓")
		}
		cursor := "    "
		if i == nav.pos {
			cursor = "  " + styleCyan.Render("❯") + " "
		}
		name := fmt.Sprintf("%-*s", nameWidth, it.name)
		if i == nav.pos {
			name = styleWhite.Render(name)
		} else {
			name = styleGray.Render(name)
		}
		buf.WriteString(cursor + statusIcon + "  " + name + "  " + it.detail + "\n")
	}

	if withContinue {
		w := m.width - 4
		if w < 20 {
			w = 60
		}
		buf.WriteString("  " + styleGray.Render(strings.Repeat("─", min(54, w))) + "\n")
		continuePrefix := "    "
		continueLabel := "Continue →"
		if nav.pos == len(items) {
			continuePrefix = "  " + styleCyan.Render("❯") + " "
			continueLabel = styleWhite.Render(continueLabel)
		} else {
			continueLabel = styleGray.Render(continueLabel)
		}
		buf.WriteString(continuePrefix + styleGreen.Render("▶") + "  " + continueLabel + "\n")
	}

	buf.WriteString("\n  " + styleGray.Render(hints) + "\n")
	buf.WriteString(m.feedback.render())
	return buf.String()
}
