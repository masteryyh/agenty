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
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const clipboardCopyNoticeDuration = 2 * time.Second

type selectableRegion int

const (
	selectableRegionNone selectableRegion = iota
	selectableRegionHistory
	selectableRegionInput
)

type selectionPoint struct {
	line int
	col  int
}

type mouseSelection struct {
	region    selectableRegion
	selecting bool
	start     selectionPoint
	end       selectionPoint
}

type clipboardCopiedMsg struct {
	err error
}

type clipboardCopyNoticeExpiredMsg struct {
	until time.Time
}

func (m *chatModel) handleMouseSelection(msg tea.MouseMsg) (tea.Cmd, bool) {
	switch msg.Action {
	case tea.MouseActionPress:
		if msg.Button != tea.MouseButtonLeft {
			return nil, false
		}
		region, point, ok := m.selectionPointForMouse(msg)
		if !ok {
			m.mouseSelection = mouseSelection{}
			return nil, true
		}
		m.mouseSelection = mouseSelection{region: region, selecting: true, start: point, end: point}
		return nil, true
	case tea.MouseActionMotion:
		if !m.mouseSelection.selecting {
			return nil, false
		}
		m.mouseSelection.end = m.clampedSelectionPoint(msg, m.mouseSelection.region)
		return nil, true
	case tea.MouseActionRelease:
		if !m.mouseSelection.selecting {
			return nil, false
		}
		m.mouseSelection.selecting = false
		m.mouseSelection.end = m.clampedSelectionPoint(msg, m.mouseSelection.region)
		text := m.selectedText()
		if strings.TrimSpace(text) == "" {
			return nil, true
		}
		return copyToClipboardCmd(text), true
	}
	return nil, false
}

func (m chatModel) renderSelectableView(view string, region selectableRegion) string {
	if m.mouseSelection.region != region || !m.mouseSelection.hasRange() {
		return view
	}
	lines := splitViewLines(view)
	for i := range lines {
		lines[i] = ansi.Strip(lines[i])
	}
	applySelectionHighlight(lines, m.mouseSelection.normalized())
	return strings.Join(lines, "\n")
}

func (m chatModel) selectedText() string {
	if !m.mouseSelection.hasRange() {
		return ""
	}
	switch m.mouseSelection.region {
	case selectableRegionHistory:
		lines := strippedViewLines(m.viewport.View())
		return selectedTextFromLines(lines, m.mouseSelection.normalized(), 0)
	case selectableRegionInput:
		lines := strippedViewLines(m.input.View())
		return selectedTextFromLines(lines, m.mouseSelection.normalized(), ansi.StringWidth(m.input.Prompt))
	default:
		return ""
	}
}

func (m chatModel) selectionPointForMouse(msg tea.MouseMsg) (selectableRegion, selectionPoint, bool) {
	if msg.Y >= 0 && msg.Y < m.viewport.Height {
		return selectableRegionHistory, selectionPoint{line: msg.Y, col: max(msg.X, 0)}, true
	}

	inputTop := m.viewport.Height + 1
	inputLines := splitViewLines(m.input.View())
	if msg.Y >= inputTop && msg.Y < inputTop+len(inputLines) {
		return selectableRegionInput, selectionPoint{line: msg.Y - inputTop, col: max(msg.X, 0)}, true
	}

	return selectableRegionNone, selectionPoint{}, false
}

func (m chatModel) clampedSelectionPoint(msg tea.MouseMsg, region selectableRegion) selectionPoint {
	switch region {
	case selectableRegionHistory:
		line := min(max(msg.Y, 0), max(m.viewport.Height-1, 0))
		return selectionPoint{line: line, col: max(msg.X, 0)}
	case selectableRegionInput:
		inputTop := m.viewport.Height + 1
		inputLines := splitViewLines(m.input.View())
		maxLine := max(len(inputLines)-1, 0)
		line := min(max(msg.Y-inputTop, 0), maxLine)
		return selectionPoint{line: line, col: max(msg.X, 0)}
	default:
		return selectionPoint{}
	}
}

func (s mouseSelection) hasRange() bool {
	return s.region != selectableRegionNone && (s.start.line != s.end.line || s.start.col != s.end.col)
}

func (s mouseSelection) normalized() mouseSelection {
	if s.start.line < s.end.line || (s.start.line == s.end.line && s.start.col <= s.end.col) {
		return s
	}
	s.start, s.end = s.end, s.start
	return s
}

func splitViewLines(view string) []string {
	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func strippedViewLines(view string) []string {
	lines := splitViewLines(view)
	for i := range lines {
		lines[i] = ansi.Strip(lines[i])
	}
	return lines
}

func selectedTextFromLines(lines []string, selection mouseSelection, minCol int) string {
	if len(lines) == 0 {
		return ""
	}
	selection = selection.normalized()
	startLine := min(max(selection.start.line, 0), len(lines)-1)
	endLine := min(max(selection.end.line, 0), len(lines)-1)

	selected := make([]string, 0, endLine-startLine+1)
	for line := startLine; line <= endLine; line++ {
		width := ansi.StringWidth(lines[line])
		startCol := 0
		endCol := width
		if line == startLine {
			startCol = selection.start.col
		}
		if line == endLine {
			endCol = selection.end.col + 1
		}
		startCol = min(max(startCol, minCol), width)
		endCol = min(max(endCol, minCol), width)
		if endCol < startCol {
			startCol, endCol = endCol, startCol
		}
		selected = append(selected, strings.TrimRight(ansi.Cut(lines[line], startCol, endCol), " "))
	}
	return strings.Join(selected, "\n")
}

func applySelectionHighlight(lines []string, selection mouseSelection) {
	selection = selection.normalized()
	startLine := min(max(selection.start.line, 0), len(lines)-1)
	endLine := min(max(selection.end.line, 0), len(lines)-1)
	for line := startLine; line <= endLine; line++ {
		width := ansi.StringWidth(lines[line])
		startCol := 0
		endCol := width
		if line == startLine {
			startCol = selection.start.col
		}
		if line == endLine {
			endCol = selection.end.col + 1
		}
		startCol = min(max(startCol, 0), width)
		endCol = min(max(endCol, 0), width)
		if endCol <= startCol {
			continue
		}
		before := ansi.Cut(lines[line], 0, startCol)
		selected := ansi.Cut(lines[line], startCol, endCol)
		after := ansi.Cut(lines[line], endCol, width)
		lines[line] = before + styleReverse.Render(selected) + after
	}
}

func copyToClipboardCmd(text string) tea.Cmd {
	return func() tea.Msg {
		return clipboardCopiedMsg{err: clipboard.WriteAll(text)}
	}
}

func clipboardCopyNoticeExpiredCmd(until time.Time) tea.Cmd {
	return tea.Tick(clipboardCopyNoticeDuration, func(time.Time) tea.Msg {
		return clipboardCopyNoticeExpiredMsg{until: until}
	})
}
