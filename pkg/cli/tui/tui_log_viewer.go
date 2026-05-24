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

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/masteryyh/agenty/pkg/utils/logger"
)

type logViewerOverlay struct {
	viewport   viewport.Model
	responseCh chan overlayResponse
	entries    []string
	builtWidth int
}

func newLogViewerOverlay(width, height int, responseCh chan overlayResponse) *logViewerOverlay {
	vpH := height - 3
	if vpH < 1 {
		vpH = 1
	}
	lv := &logViewerOverlay{
		viewport:   viewport.New(width, vpH),
		responseCh: responseCh,
		entries:    logger.GetStoredLogs(),
	}
	lv.buildContent(width)
	lv.viewport.GotoBottom()
	return lv
}

func (l *logViewerOverlay) buildContent(width int) {
	w := max(width-4, 20)
	if l.builtWidth == w {
		return
	}
	l.builtWidth = w

	var sb strings.Builder
	if len(l.entries) == 0 {
		sb.WriteString("  No debug logs captured yet.\n")
	} else {
		for _, e := range l.entries {
			sb.WriteString(WrapForDisplay(e))
		}
	}
	atBottom := l.viewport.AtBottom()
	l.viewport.SetContent(sb.String())
	if atBottom {
		l.viewport.GotoBottom()
	}
}

func (l *logViewerOverlay) handleKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		l.responseCh <- overlayResponse{}
		return true, nil
	case tea.KeyRunes:
		if string(msg.Runes) == "q" {
			l.responseCh <- overlayResponse{}
			return true, nil
		}
	case tea.KeyUp:
		l.viewport.LineUp(1)
	case tea.KeyDown:
		l.viewport.LineDown(1)
	case tea.KeyPgUp:
		l.viewport.HalfViewUp()
	case tea.KeyPgDown:
		l.viewport.HalfViewDown()
	case tea.KeyHome:
		l.viewport.GotoTop()
	case tea.KeyEnd:
		l.viewport.GotoBottom()
	}
	return false, nil
}

func (l *logViewerOverlay) handleMouse(msg tea.MouseMsg) tea.Cmd {
	var cmd tea.Cmd
	l.viewport, cmd = l.viewport.Update(msg)
	return cmd
}

func (l *logViewerOverlay) render(width, height int) string {
	vpH := height - 3
	if vpH < 1 {
		vpH = 1
	}
	l.viewport.Width = width
	l.viewport.Height = vpH
	l.buildContent(width)

	title := styleBold.Render("debug logs")
	scrollPct := fmt.Sprintf("%3d%%", int(l.viewport.ScrollPercent()*100))
	scrollInfo := styleGray.Render(scrollPct)
	titleWidth := lipgloss.Width(title)
	scrollWidth := lipgloss.Width(scrollInfo)
	dashCount := width - titleWidth - scrollWidth - 6
	if dashCount < 0 {
		dashCount = 0
	}
	headerLine := "  " + title + "  " + styleBarSep.Render(strings.Repeat("─", dashCount)) + "  " + scrollInfo

	footerSep := styleBarSep.Render(strings.Repeat("─", width))
	hints := "  " + styleGray.Render("q/Esc close  ·  ↑/↓ scroll  ·  PgUp/PgDn half page  ·  Home/End top/bottom")

	var buf strings.Builder
	buf.WriteString(headerLine)
	buf.WriteString("\n")
	buf.WriteString(l.viewport.View())
	buf.WriteString("\n")
	buf.WriteString(footerSep)
	buf.WriteString("\n")
	buf.WriteString(hints)
	return buf.String()
}
