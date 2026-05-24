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

package wizard

import tea "github.com/charmbracelet/bubbletea"

const listMaxVisible = 12

type selectionModel struct {
	pos        int
	max        int
	offset     int
	maxVisible int
	selected   []int
	maxSelect  int
}

func newSelectionModel(maxPos int) selectionModel {
	return selectionModel{max: maxPos, maxVisible: listMaxVisible}
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
