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

import "github.com/charmbracelet/lipgloss"

var (
	Bold    lipgloss.Style
	Italic  lipgloss.Style
	Reverse lipgloss.Style

	Primary  lipgloss.Style
	Accent   lipgloss.Style
	Muted    lipgloss.Style
	Subtle   lipgloss.Style
	Faint    lipgloss.Style
	Dim      lipgloss.Style
	White    lipgloss.Style
	Green    lipgloss.Style
	Red      lipgloss.Style
	Yellow   lipgloss.Style
	Cyan     lipgloss.Style
	Blue     lipgloss.Style
	MagentaS lipgloss.Style

	Sep             lipgloss.Style
	AssistantHeader lipgloss.Style
	UserHeader      lipgloss.Style
	Timestamp       lipgloss.Style
	ModelInfo       lipgloss.Style
	Content         lipgloss.Style
	Reasoning       lipgloss.Style
	ReasoningLabel  lipgloss.Style
	ToolLabel       lipgloss.Style
	ToolName        lipgloss.Style
	ToolArgs        lipgloss.Style
	ToolSuccess     lipgloss.Style
	ToolError       lipgloss.Style
	ToolResult      lipgloss.Style
	FinalLabel      lipgloss.Style
	SysMsg          lipgloss.Style
	SysOK           lipgloss.Style
	SysErr          lipgloss.Style

	InputPromptFocused lipgloss.Style
	InputPromptBlurred lipgloss.Style
	InputText          lipgloss.Style
	InputPlaceholder   lipgloss.Style

	Spinner    lipgloss.Style
	SpinnerTxt lipgloss.Style

	BarSep    lipgloss.Style
	BarModel  lipgloss.Style
	BarThink  lipgloss.Style
	HintMuted lipgloss.Style
	Streaming lipgloss.Style
)

func init() {
	initStyles()
}

func initStyles() {
	Bold = lipgloss.NewStyle().Bold(true)
	Italic = lipgloss.NewStyle().Italic(true)
	Reverse = lipgloss.NewStyle().Reverse(true)

	Primary = lipgloss.NewStyle().Foreground(Colors.Primary)
	Accent = lipgloss.NewStyle().Foreground(Colors.Accent)
	Muted = lipgloss.NewStyle().Foreground(Colors.TextMuted)
	Subtle = lipgloss.NewStyle().Foreground(Colors.TextSubtle)
	Faint = lipgloss.NewStyle().Foreground(Colors.TextFaint)
	White = lipgloss.NewStyle().Foreground(Colors.Text)
	Green = lipgloss.NewStyle().Foreground(Colors.Success)
	Red = lipgloss.NewStyle().Foreground(Colors.Error)
	Yellow = lipgloss.NewStyle().Foreground(Colors.Warning)
	Cyan = lipgloss.NewStyle().Foreground(Colors.Primary)
	Blue = lipgloss.NewStyle().Foreground(Colors.Secondary)
	MagentaS = lipgloss.NewStyle().Foreground(Colors.Magenta)

	Sep = lipgloss.NewStyle().Foreground(Colors.TextSubtle)
	AssistantHeader = lipgloss.NewStyle().Foreground(Colors.Primary).Bold(true)
	UserHeader = lipgloss.NewStyle().Foreground(Colors.Secondary).Bold(true)
	Timestamp = lipgloss.NewStyle().Foreground(Colors.TextMuted)
	Content = lipgloss.NewStyle().Foreground(Colors.Text).Italic(false)
	Reasoning = lipgloss.NewStyle().Foreground(Colors.TextFaint).Italic(true)
	ReasoningLabel = lipgloss.NewStyle().Foreground(Colors.Special).Bold(true)
	ToolLabel = lipgloss.NewStyle().Foreground(Colors.Accent).Bold(true)
	ToolName = lipgloss.NewStyle().Foreground(Colors.Highlight)
	ToolArgs = lipgloss.NewStyle().Foreground(Colors.TextFaint)
	ToolSuccess = lipgloss.NewStyle().Foreground(Colors.Success)
	ToolError = lipgloss.NewStyle().Foreground(Colors.Error)
	ToolResult = lipgloss.NewStyle().Foreground(Colors.TextMuted)
	FinalLabel = lipgloss.NewStyle().Foreground(Colors.Primary).Bold(true)
	SysMsg = lipgloss.NewStyle().Foreground(Colors.TextMuted)
	SysOK = lipgloss.NewStyle().Foreground(Colors.Success)
	SysErr = lipgloss.NewStyle().Foreground(Colors.Error)

	InputPromptBlurred = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	InputText = lipgloss.NewStyle().Foreground(Colors.Text)
	InputPlaceholder = lipgloss.NewStyle().Foreground(Colors.TextFaint)

	BarSep = lipgloss.NewStyle().Foreground(Colors.TextSubtle)
	BarModel = lipgloss.NewStyle().Foreground(Colors.Primary).Bold(true)
	BarThink = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))

	if IsDark {
		Dim = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
		ModelInfo = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
		InputPromptFocused = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
		HintMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("237"))
		Streaming = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
		Spinner = lipgloss.NewStyle().Foreground(lipgloss.Color("213"))
		SpinnerTxt = lipgloss.NewStyle().Foreground(lipgloss.Color("219"))
	} else {
		Dim = lipgloss.NewStyle().Foreground(Colors.TextFaint)
		ModelInfo = lipgloss.NewStyle().Foreground(Colors.TextMuted)
		InputPromptFocused = lipgloss.NewStyle().Foreground(Colors.Primary)
		HintMuted = lipgloss.NewStyle().Foreground(Colors.TextFaint)
		Streaming = lipgloss.NewStyle().Foreground(lipgloss.Color("136"))
		Spinner = lipgloss.NewStyle().Foreground(Colors.Primary)
		SpinnerTxt = lipgloss.NewStyle().Foreground(Colors.Secondary)
	}
}
