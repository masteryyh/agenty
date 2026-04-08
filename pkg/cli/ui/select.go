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

func SelectOption(title string, options []string, defaultIndex int) (int, error) {
	if defaultIndex < 0 || defaultIndex >= len(options) {
		defaultIndex = 0
	}

	raw, err := EnterRawMode()
	if err != nil {
		return -1, err
	}
	defer raw.Restore()

	cursor := defaultIndex

	render := func() int {
		lines := 0
		HideCursor()
		Writeln("  " + pterm.Bold.Sprint(title))
		lines++
		Writeln("  " + pterm.FgGray.Sprint(strings.Repeat("─", 40)))
		lines++
		Writeln("")
		lines++

		for i, opt := range options {
			if i == cursor {
				Writeln("  " + pterm.FgCyan.Sprint("❯") + " " + pterm.FgWhite.Sprint(opt))
			} else {
				Writeln("    " + pterm.FgGray.Sprint(opt))
			}
			lines++
		}

		Writeln("")
		lines++
		Write("  " + pterm.FgGray.Sprint("↑/↓ navigate  ·  Enter select  ·  Esc back"))
		lines++
		return lines
	}

	renderedLines := render()

	buf := make([]byte, 64)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			ExitClean(renderedLines)
			return -1, err
		}

		if n >= 3 && buf[0] == 27 && buf[1] == 91 {
			switch buf[2] {
			case 65:
				if cursor > 0 {
					cursor--
				}
				ClearLines(renderedLines)
				renderedLines = render()
				continue
			case 66:
				if cursor < len(options)-1 {
					cursor++
				}
				ClearLines(renderedLines)
				renderedLines = render()
				continue
			}
		}

		if n == 1 {
			switch buf[0] {
			case 13:
				ExitClean(renderedLines)
				return cursor, nil
			case 27:
				ExitClean(renderedLines)
				return -1, ErrCancelled
			}
		}
	}
}
