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
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/masteryyh/agenty/pkg/cli/theme"
)

const wizMaxModels = 4

type wizInput struct {
	model textinput.Model
	label string
}

func newWizTextInput(masked bool) textinput.Model {
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
	ti := newWizTextInput(masked)
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

type wizFeedback struct {
	msg  string
	kind feedbackKind
}

func (f wizFeedback) setOK(msg string) wizFeedback { return wizFeedback{msg: msg, kind: feedbackOK} }
func (f wizFeedback) setWarn(msg string) wizFeedback {
	return wizFeedback{msg: msg, kind: feedbackWarn}
}
func (f wizFeedback) setErr(msg string) wizFeedback { return wizFeedback{msg: msg, kind: feedbackErr} }
func (wizFeedback) clear() wizFeedback              { return wizFeedback{} }

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
