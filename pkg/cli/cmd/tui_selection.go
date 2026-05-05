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

import tea "github.com/charmbracelet/bubbletea"

type selectionModel struct {
	pos        int
	max        int
	offset     int
	maxVisible int
	selected   []int
	maxSelect  int
}

func newSelectionModel(max int) selectionModel {
	return selectionModel{max: max, maxVisible: listMaxVisible}
}

func (s selectionModel) withCursor(cursor int) selectionModel {
	s.pos = min(max(cursor, 0), max(s.max, 0))
	if s.maxVisible > 0 && s.pos >= s.maxVisible {
		s.offset = s.pos - s.maxVisible + 1
	}
	return s
}

func (s selectionModel) withMultiSelect(selected []int, maxSelect int) selectionModel {
	s.selected = append([]int(nil), selected...)
	s.maxSelect = maxSelect
	return s
}

func (s selectionModel) Up() selectionModel {
	if s.pos > 0 {
		s.pos--
		if s.pos < s.offset {
			s.offset = s.pos
		}
	}
	return s
}

func (s selectionModel) Down() selectionModel {
	if s.pos < s.max {
		s.pos++
		if s.maxVisible > 0 && s.pos >= s.offset+s.maxVisible {
			s.offset = s.pos - s.maxVisible + 1
		}
	}
	return s
}

func (s selectionModel) HandleNavKey(msg tea.KeyMsg) (selectionModel, bool) {
	switch msg.Type {
	case tea.KeyUp:
		return s.Up(), true
	case tea.KeyDown:
		return s.Down(), true
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "k":
			return s.Up(), true
		case "j":
			return s.Down(), true
		}
	}
	return s, false
}

func (s selectionModel) selectionPos(idx int) int {
	for i, sel := range s.selected {
		if sel == idx {
			return i
		}
	}
	return -1
}

func (s selectionModel) toggle(idx int) selectionModel {
	for i, sel := range s.selected {
		if sel == idx {
			s.selected = append(s.selected[:i], s.selected[i+1:]...)
			return s
		}
	}
	if s.maxSelect <= 0 || len(s.selected) < s.maxSelect {
		s.selected = append(s.selected, idx)
	}
	return s
}
