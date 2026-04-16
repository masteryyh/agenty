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
	Bold    = lipgloss.NewStyle().Bold(true)
	Italic  = lipgloss.NewStyle().Italic(true)
	Reverse = lipgloss.NewStyle().Reverse(true)

	Primary  = lipgloss.NewStyle().Foreground(Colors.Primary)
	Accent   = lipgloss.NewStyle().Foreground(Colors.Accent)
	Muted    = lipgloss.NewStyle().Foreground(Colors.TextMuted)
	Subtle   = lipgloss.NewStyle().Foreground(Colors.TextSubtle)
	Faint    = lipgloss.NewStyle().Foreground(Colors.TextFaint)
	Dim      = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	White    = lipgloss.NewStyle().Foreground(Colors.Text)
	Green    = lipgloss.NewStyle().Foreground(Colors.Success)
	Red      = lipgloss.NewStyle().Foreground(Colors.Error)
	Yellow   = lipgloss.NewStyle().Foreground(Colors.Warning)
	Cyan     = lipgloss.NewStyle().Foreground(Colors.Primary)
	Blue     = lipgloss.NewStyle().Foreground(Colors.Secondary)
	MagentaS = lipgloss.NewStyle().Foreground(Colors.Magenta)

	Sep             = lipgloss.NewStyle().Foreground(Colors.TextSubtle)
	AssistantHeader = lipgloss.NewStyle().Foreground(Colors.Primary).Bold(true)
	UserHeader      = lipgloss.NewStyle().Foreground(Colors.Secondary).Bold(true)
	Timestamp       = lipgloss.NewStyle().Foreground(Colors.TextMuted)
	ModelInfo       = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	Content         = lipgloss.NewStyle().Foreground(Colors.Text)
	Reasoning       = lipgloss.NewStyle().Foreground(Colors.TextFaint).Italic(true)
	ReasoningLabel  = lipgloss.NewStyle().Foreground(Colors.Special).Bold(true)
	ToolLabel       = lipgloss.NewStyle().Foreground(Colors.Accent).Bold(true)
	ToolName        = lipgloss.NewStyle().Foreground(Colors.Highlight)
	ToolArgs        = lipgloss.NewStyle().Foreground(Colors.TextFaint)
	ToolSuccess     = lipgloss.NewStyle().Foreground(Colors.Success)
	ToolError       = lipgloss.NewStyle().Foreground(Colors.Error)
	ToolResult      = lipgloss.NewStyle().Foreground(Colors.TextMuted)
	FinalLabel      = lipgloss.NewStyle().Foreground(Colors.Primary).Bold(true)
	SysMsg          = lipgloss.NewStyle().Foreground(Colors.TextMuted)
	SysOK           = lipgloss.NewStyle().Foreground(Colors.Success)
	SysErr          = lipgloss.NewStyle().Foreground(Colors.Error)

	InputPromptFocused = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	InputPromptBlurred = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	InputText          = lipgloss.NewStyle().Foreground(Colors.Text)
	InputPlaceholder   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	Spinner    = lipgloss.NewStyle().Foreground(lipgloss.Color("213"))
	SpinnerTxt = lipgloss.NewStyle().Foreground(lipgloss.Color("219"))

	BarSep    = lipgloss.NewStyle().Foreground(Colors.TextSubtle)
	BarModel  = lipgloss.NewStyle().Foreground(Colors.Primary).Bold(true)
	BarThink  = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	HintMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("237"))
	Streaming = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
)
