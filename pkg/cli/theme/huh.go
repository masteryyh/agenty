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

package theme

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

func NewHuhTheme() *huh.Theme {
	t := huh.ThemeBase()

	buttonFg := lipgloss.Color("0")
	blurredBtnBg := lipgloss.Color("236")
	if !IsDark {
		buttonFg = lipgloss.Color("255")
		blurredBtnBg = lipgloss.Color("250")
	}

	t.Focused.Base = lipgloss.NewStyle().
		PaddingLeft(1).
		BorderStyle(lipgloss.ThickBorder()).
		BorderLeft(true).
		BorderForeground(Colors.Primary)
	t.Focused.Title = lipgloss.NewStyle().Foreground(Colors.Text).Bold(true)
	t.Focused.Description = lipgloss.NewStyle().Foreground(Colors.TextMuted)
	t.Focused.ErrorIndicator = lipgloss.NewStyle().Foreground(Colors.Error).SetString(" ✗")
	t.Focused.ErrorMessage = lipgloss.NewStyle().Foreground(Colors.Error)
	t.Focused.SelectSelector = lipgloss.NewStyle().Foreground(Colors.Primary).SetString("❯ ")
	t.Focused.NextIndicator = lipgloss.NewStyle().Foreground(Colors.Primary).MarginLeft(1).SetString("↓")
	t.Focused.PrevIndicator = lipgloss.NewStyle().Foreground(Colors.Primary).MarginRight(1).SetString("↑")
	t.Focused.Option = lipgloss.NewStyle().Foreground(Colors.Text)
	t.Focused.SelectedOption = lipgloss.NewStyle().Foreground(Colors.Primary)
	t.Focused.UnselectedOption = lipgloss.NewStyle().Foreground(Colors.TextMuted)
	t.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(Colors.Primary).SetString("[●] ")
	t.Focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(Colors.TextMuted).SetString("[ ] ")
	t.Focused.MultiSelectSelector = lipgloss.NewStyle().Foreground(Colors.Primary).SetString("❯ ")
	t.Focused.FocusedButton = lipgloss.NewStyle().
		Padding(0, 2).MarginRight(1).
		Foreground(buttonFg).Background(Colors.Primary)
	t.Focused.BlurredButton = lipgloss.NewStyle().
		Padding(0, 2).MarginRight(1).
		Foreground(Colors.TextMuted).Background(blurredBtnBg)
	t.Focused.TextInput.Cursor = lipgloss.NewStyle().Foreground(Colors.Primary)
	t.Focused.TextInput.CursorText = lipgloss.NewStyle().Foreground(Colors.Text).Background(Colors.Primary)
	t.Focused.TextInput.Text = lipgloss.NewStyle().Foreground(Colors.Text)
	t.Focused.TextInput.Placeholder = lipgloss.NewStyle().Foreground(Colors.TextFaint)
	t.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(Colors.Primary).SetString("▸ ")
	t.Focused.NoteTitle = lipgloss.NewStyle().Foreground(Colors.Accent).Bold(true)

	t.Blurred = t.Focused
	t.Blurred.Base = lipgloss.NewStyle().
		PaddingLeft(2).
		BorderStyle(lipgloss.HiddenBorder()).
		BorderLeft(true)
	t.Blurred.Title = lipgloss.NewStyle().Foreground(Colors.TextSubtle)
	t.Blurred.Description = lipgloss.NewStyle().Foreground(Colors.TextSubtle)
	t.Blurred.SelectSelector = lipgloss.NewStyle().SetString("  ")
	t.Blurred.MultiSelectSelector = lipgloss.NewStyle().SetString("  ")
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()
	t.Blurred.TextInput.Prompt = lipgloss.NewStyle().Foreground(Colors.TextSubtle).SetString("  ")
	t.Blurred.FocusedButton = lipgloss.NewStyle().
		Padding(0, 2).MarginRight(1).
		Foreground(Colors.TextSubtle).Background(blurredBtnBg)

	t.Group.Title = lipgloss.NewStyle().Foreground(Colors.Accent).Bold(true).MarginBottom(1)
	t.Group.Description = lipgloss.NewStyle().Foreground(Colors.TextMuted).MarginBottom(1)
	t.FieldSeparator = lipgloss.NewStyle().SetString("\n")

	return t
}

func NewHuhKeyMap() *huh.KeyMap {
	k := huh.NewDefaultKeyMap()

	k.Select.Up = key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up"))
	k.Select.Down = key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down"))
	k.MultiSelect.Up = key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up"))
	k.MultiSelect.Down = key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down"))
	k.FilePicker.Up = key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up"), key.WithDisabled())
	k.FilePicker.Down = key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down"), key.WithDisabled())

	return k
}
