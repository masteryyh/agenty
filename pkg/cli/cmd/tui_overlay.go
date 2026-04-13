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
	title      string
	items      []string
	hints      string
	cursor     int
	offset     int
	responseCh chan overlayResponse
}

func newListOverlay(title string, items []string, hints string, ch chan overlayResponse) *listOverlay {
	return &listOverlay{title: title, items: items, hints: hints, responseCh: ch}
}

func (l *listOverlay) handleKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyUp:
		if l.cursor > 0 {
			l.cursor--
			if l.cursor < l.offset {
				l.offset = l.cursor
			}
		}
	case tea.KeyDown:
		if l.cursor < len(l.items)-1 {
			l.cursor++
			if l.cursor >= l.offset+listMaxVisible {
				l.offset = l.cursor - listMaxVisible + 1
			}
		}
	case tea.KeyEnter:
		l.responseCh <- overlayResponse{listAction: ListActionSelect, listIndex: l.cursor}
		return true
	case tea.KeyEsc, tea.KeyCtrlC:
		l.responseCh <- overlayResponse{listAction: ListActionCancel, listIndex: -1}
		return true
	case tea.KeyCtrlD:
		l.responseCh <- overlayResponse{listAction: ListActionDelete, listIndex: l.cursor}
		return true
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "a", "A":
			l.responseCh <- overlayResponse{listAction: ListActionAdd, listIndex: l.cursor}
			return true
		case "e", "E":
			l.responseCh <- overlayResponse{listAction: ListActionEdit, listIndex: l.cursor}
			return true
		}
	}
	return false
}

func (l *listOverlay) render(width, _ int) string {
	var buf strings.Builder
	sep := max(min(56, width-4), 10)
	fmt.Fprintf(&buf, "\n  %s\n", styleBold.Render(l.title))
	fmt.Fprintf(&buf, "  %s\n\n", styleGray.Render(strings.Repeat("─", sep)))

	end := min(l.offset + listMaxVisible, len(l.items))
	for i := l.offset; i < end; i++ {
		if i == l.cursor {
			fmt.Fprintf(&buf, "  %s %s\n", styleCyan.Render("❯"), l.items[i])
		} else {
			fmt.Fprintf(&buf, "    %s\n", l.items[i])
		}
	}

	if len(l.items) > listMaxVisible {
		fmt.Fprintf(&buf, "\n  %s\n", styleGray.Render(fmt.Sprintf("(%d/%d)", l.cursor+1, len(l.items))))
	}
	fmt.Fprintf(&buf, "\n  %s\n", styleGray.Render(l.hints))
	return buf.String()
}

type multiSelectOverlay struct {
	title      string
	options    []string
	cursor     int
	selected   []int
	responseCh chan overlayResponse
}

func newMultiSelectOverlay(title string, options []string, defaultSelected []int, ch chan overlayResponse) *multiSelectOverlay {
	selected := make([]int, 0, len(defaultSelected))
	selected = append(selected, defaultSelected...)
	return &multiSelectOverlay{title: title, options: options, selected: selected, responseCh: ch}
}

func (s *multiSelectOverlay) selectionPos(idx int) int {
	for i, sel := range s.selected {
		if sel == idx {
			return i
		}
	}
	return -1
}

func (s *multiSelectOverlay) toggle(idx int) {
	for i, sel := range s.selected {
		if sel == idx {
			s.selected = append(s.selected[:i], s.selected[i+1:]...)
			return
		}
	}
	if len(s.selected) >= wizMaxModels {
		return
	}
	s.selected = append(s.selected, idx)
}

func (s *multiSelectOverlay) handleKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyUp:
		if s.cursor > 0 {
			s.cursor--
		}
	case tea.KeyDown:
		if s.cursor < len(s.options)-1 {
			s.cursor++
		}
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "k":
			if s.cursor > 0 {
				s.cursor--
			}
		case "j":
			if s.cursor < len(s.options)-1 {
				s.cursor++
			}
		case " ":
			s.toggle(s.cursor)
		}
	case tea.KeySpace:
		s.toggle(s.cursor)
	case tea.KeyEnter:
		if len(s.selected) > 0 {
			s.responseCh <- overlayResponse{selectedIndices: s.selected}
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
		selPos := s.selectionPos(i)
		cursor := "  "
		if i == s.cursor {
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
			if i == s.cursor {
				label = styleWhite.Render(opt)
			} else {
				label = styleGray.Render(opt)
			}
		}
		buf.WriteString(fmt.Sprintf("  %s%s  %s\n", cursor, badge, label))
	}

	buf.WriteString("\n")
	if len(s.selected) == 0 {
		buf.WriteString(fmt.Sprintf("  %s  %s\n", styleYellow.Render("⚠"), styleGray.Render("Select at least one model")))
	} else if len(s.selected) == 1 {
		buf.WriteString(fmt.Sprintf("  %s  %s %s  %s\n",
			styleGreen.Render("✓"),
			styleYellow.Render("★"),
			styleWhite.Render(s.options[s.selected[0]]),
			styleGray.Render("(primary)")))
	} else {
		buf.WriteString(fmt.Sprintf("  %s  %s %s  %s\n",
			styleGreen.Render("✓"),
			styleYellow.Render("★"),
			styleWhite.Render(s.options[s.selected[0]]),
			styleGray.Render(fmt.Sprintf("+ %d fallback(s)", len(s.selected)-1))))
	}

	buf.WriteString(fmt.Sprintf("\n  %s\n",
		styleGray.Render("↑/↓ navigate  ·  Space select  ·  Enter confirm  ·  Esc back")))
	return buf.String()
}
