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

package ui

import (
	"os"
	"strings"

	"github.com/pterm/pterm"
)

type ListAction int

const (
	ListActionSelect ListAction = iota
	ListActionAdd
	ListActionEdit
	ListActionDelete
	ListActionCancel
)

type ListResult struct {
	Action ListAction
	Index  int
}

func ShowList(title string, items []string, hints string) (*ListResult, error) {
	if len(items) == 0 {
		return &ListResult{Action: ListActionCancel, Index: -1}, nil
	}

	raw, err := EnterRawMode()
	if err != nil {
		return nil, err
	}
	defer raw.Restore()

	cursor := 0
	maxVisible := 10
	offset := 0

	render := func() int {
		lines := 0
		HideCursor()

		Writeln("  " + pterm.Bold.Sprint(title))
		lines++
		Writeln("  " + pterm.FgGray.Sprint(strings.Repeat("─", 56)))
		lines++
		Writeln("")
		lines++

		visibleEnd := min(offset+maxVisible, len(items))

		for i := offset; i < visibleEnd; i++ {
			if i == cursor {
				Writeln("  " + pterm.FgCyan.Sprint("❯") + " " + pterm.FgWhite.Sprint(items[i]))
			} else {
				Writeln("    " + pterm.FgGray.Sprint(items[i]))
			}
			lines++
		}

		if len(items) > maxVisible {
			Writeln("  " + pterm.FgGray.Sprintf("(%d/%d)", cursor+1, len(items)))
			lines++
		}

		Writeln("")
		lines++
		Write("  " + pterm.FgGray.Sprint(hints))
		lines++

		return lines
	}

	renderedLines := render()

	buf := make([]byte, 64)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			ExitClean(renderedLines)
			return nil, err
		}

		if n >= 3 && buf[0] == 27 && buf[1] == 91 {
			switch buf[2] {
			case 65: // Up
				if cursor > 0 {
					cursor--
					if cursor < offset {
						offset = cursor
					}
				}
				ClearLines(renderedLines)
				renderedLines = render()
				continue
			case 66: // Down
				if cursor < len(items)-1 {
					cursor++
					if cursor >= offset+maxVisible {
						offset = cursor - maxVisible + 1
					}
				}
				ClearLines(renderedLines)
				renderedLines = render()
				continue
			}
		}

		if n == 1 {
			switch buf[0] {
			case 27: // Esc
				ExitClean(renderedLines)
				return &ListResult{Action: ListActionCancel, Index: -1}, nil
			case 13: // Enter
				ExitClean(renderedLines)
				return &ListResult{Action: ListActionSelect, Index: cursor}, nil
			case 'a', 'A':
				ExitClean(renderedLines)
				return &ListResult{Action: ListActionAdd, Index: cursor}, nil
			case 'e', 'E':
				ExitClean(renderedLines)
				return &ListResult{Action: ListActionEdit, Index: cursor}, nil
			case 4: // Ctrl+D
				ExitClean(renderedLines)
				return &ListResult{Action: ListActionDelete, Index: cursor}, nil
			}
		}
	}
}
