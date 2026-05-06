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

	tea "github.com/charmbracelet/bubbletea"
)

const listMaxVisible = 12

type listOverlay struct {
	title         string
	subtitle      string
	items         []string
	hints         string
	notice        string
	validate      func(action ListAction, idx int) error
	deleteConfirm func(idx int) string
	confirmDelete bool
	confirmText   string
	selection     selectionModel
	responseCh    chan overlayResponse
}

func newListOverlay(title string, items []string, hints string, cursor int, validate func(action ListAction, idx int) error, deleteConfirm func(idx int) string, ch chan overlayResponse) *listOverlay {
	return &listOverlay{
		title:         title,
		items:         items,
		hints:         hints,
		validate:      validate,
		deleteConfirm: deleteConfirm,
		selection:     newSelectionModel(len(items) - 1).withCursor(cursor),
		responseCh:    ch,
	}
}

func (l *listOverlay) emitAction(action ListAction) bool {
	if l.validate != nil {
		if err := l.validate(action, l.selection.pos); err != nil {
			l.notice = err.Error()
			return false
		}
	}
	l.responseCh <- overlayResponse{listAction: action, listIndex: l.selection.pos}
	return true
}

func (l *listOverlay) handleKey(msg tea.KeyMsg) bool {
	if selection, ok := l.selection.HandleNavKey(msg); ok {
		l.selection = selection
		l.notice = ""
		l.confirmDelete = false
		l.confirmText = ""
		return false
	}
	if l.confirmDelete {
		switch msg.Type {
		case tea.KeyEsc, tea.KeyCtrlC:
			l.confirmDelete = false
			l.confirmText = ""
			return false
		case tea.KeyEnter:
			l.responseCh <- overlayResponse{listAction: ListActionDelete, listIndex: l.selection.pos}
			return true
		case tea.KeyRunes:
			switch string(msg.Runes) {
			case "y", "Y":
				l.responseCh <- overlayResponse{listAction: ListActionDelete, listIndex: l.selection.pos}
				return true
			case "n", "N":
				l.confirmDelete = false
				l.confirmText = ""
				return false
			}
		}
		return false
	}
	switch msg.Type {
	case tea.KeyEnter:
		return l.emitAction(ListActionSelect)
	case tea.KeyEsc, tea.KeyCtrlC:
		l.responseCh <- overlayResponse{listAction: ListActionCancel, listIndex: -1}
		return true
	case tea.KeyCtrlD:
		if l.deleteConfirm != nil {
			if prompt := l.deleteConfirm(l.selection.pos); prompt != "" {
				l.notice = ""
				l.confirmDelete = true
				l.confirmText = prompt
				return false
			}
		}
		return l.emitAction(ListActionDelete)
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "a", "A":
			return l.emitAction(ListActionAdd)
		case "e", "E":
			return l.emitAction(ListActionEdit)
		}
	}
	return false
}

func (l *listOverlay) render(width, _ int) string {
	var buf strings.Builder
	sep := max(min(56, width-4), 10)
	fmt.Fprintf(&buf, "\n  %s\n", styleBold.Render(l.title))
	fmt.Fprintf(&buf, "  %s\n", styleGray.Render(strings.Repeat("─", sep)))
	if l.subtitle != "" {
		for line := range strings.SplitSeq(l.subtitle, "\n") {
			fmt.Fprintf(&buf, "  %s\n", line)
		}
	}
	buf.WriteString("\n")

	end := min(l.selection.offset+listMaxVisible, len(l.items))
	for i := l.selection.offset; i < end; i++ {
		if i == l.selection.pos {
			fmt.Fprintf(&buf, "  %s %s\n", styleCyan.Render("❯"), l.items[i])
		} else {
			fmt.Fprintf(&buf, "    %s\n", l.items[i])
		}
	}

	if len(l.items) > listMaxVisible {
		fmt.Fprintf(&buf, "\n  %s\n", styleGray.Render(fmt.Sprintf("(%d/%d)", l.selection.pos+1, len(l.items))))
	}
	if l.confirmDelete && l.confirmText != "" {
		fmt.Fprintf(&buf, "\n  %s %s\n", styleYellow.Render("?"), styleYellow.Render(l.confirmText))
		fmt.Fprintf(&buf, "\n  %s\n", styleGray.Render("y confirm  ·  n cancel  ·  Esc cancel"))
		return buf.String()
	}
	if l.notice != "" {
		fmt.Fprintf(&buf, "\n  %s %s\n", styleRed.Render("!"), styleRed.Render(l.notice))
	}
	fmt.Fprintf(&buf, "\n  %s\n", styleGray.Render(l.hints))
	return buf.String()
}

type multiSelectOverlay struct {
	title      string
	options    []string
	selection  selectionModel
	responseCh chan overlayResponse
}

func newMultiSelectOverlay(title string, options []string, defaultSelected []int, ch chan overlayResponse) *multiSelectOverlay {
	selection := newSelectionModel(len(options)-1).withMultiSelect(defaultSelected, wizMaxModels)
	return &multiSelectOverlay{title: title, options: options, selection: selection, responseCh: ch}
}

func (s *multiSelectOverlay) handleKey(msg tea.KeyMsg) bool {
	if selection, ok := s.selection.HandleNavKey(msg); ok {
		s.selection = selection
		return false
	}
	switch msg.Type {
	case tea.KeyRunes:
		if string(msg.Runes) == " " {
			s.selection = s.selection.toggle(s.selection.pos)
		}
	case tea.KeySpace:
		s.selection = s.selection.toggle(s.selection.pos)
	case tea.KeyEnter:
		if len(s.selection.selected) > 0 {
			s.responseCh <- overlayResponse{selectedIndices: s.selection.selected}
			return true
		}
	case tea.KeyEsc, tea.KeyCtrlC:
		s.responseCh <- overlayResponse{selectedIndices: nil}
		return true
	}
	return false
}

func (s *multiSelectOverlay) render(width, _ int) string {
	var buf strings.Builder
	sep := min(56, width-4)
	if sep < 10 {
		sep = 10
	}
	buf.WriteString(fmt.Sprintf("\n  %s\n", styleBold.Render(s.title)))
	buf.WriteString(fmt.Sprintf("  %s\n\n", styleGray.Render(strings.Repeat("─", sep))))

	for i, opt := range s.options {
		selPos := s.selection.selectionPos(i)
		cursor := "  "
		if i == s.selection.pos {
			cursor = styleCyan.Render("❯") + " "
		}
		var badge string
		var label string
		if selPos == 0 {
			badge = styleYellow.Render("★")
			label = styleWhite.Render(opt)
		} else if selPos > 0 {
			badge = styleCyan.Render(fmt.Sprintf("%d", selPos+1))
			label = styleWhite.Render(opt)
		} else {
			badge = styleGray.Render("○")
			if i == s.selection.pos {
				label = styleWhite.Render(opt)
			} else {
				label = styleGray.Render(opt)
			}
		}
		buf.WriteString(fmt.Sprintf("  %s%s  %s\n", cursor, badge, label))
	}

	buf.WriteString("\n")
	if len(s.selection.selected) == 0 {
		buf.WriteString(fmt.Sprintf("  %s  %s\n", styleYellow.Render("⚠"), styleGray.Render("Select at least one model")))
	} else if len(s.selection.selected) == 1 {
		buf.WriteString(fmt.Sprintf("  %s  %s %s  %s\n",
			styleGreen.Render("✓"),
			styleYellow.Render("★"),
			styleWhite.Render(s.options[s.selection.selected[0]]),
			styleGray.Render("(primary)")))
	} else {
		buf.WriteString(fmt.Sprintf("  %s  %s %s  %s\n",
			styleGreen.Render("✓"),
			styleYellow.Render("★"),
			styleWhite.Render(s.options[s.selection.selected[0]]),
			styleGray.Render(fmt.Sprintf("+ %d fallback(s)", len(s.selection.selected)-1))))
	}

	buf.WriteString(fmt.Sprintf("\n  %s\n",
		styleGray.Render("↑/↓ navigate  ·  Space select  ·  Enter confirm  ·  Esc back")))
	return buf.String()
}
