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
	"fmt"
	"os"
	"strings"

	"github.com/pterm/pterm"
)

type FormFieldType int

const (
	FormFieldText     FormFieldType = iota
	FormFieldPassword               // masked
	FormFieldSelect                 // cycle with ←/→
	FormFieldToggle                 // yes/no
)

type FormField struct {
	Label       string
	Type        FormFieldType
	Value       string   // Text/Password: initial and final value
	Options     []string // Select/Toggle: option labels
	SelIdx      int      // Select/Toggle: current index
	Required    bool
	Placeholder string
	OnChange    func(selIdx int, inputs [][]rune, update func(fieldIdx int, val string))
	VisibleWhen func(fields []*FormField) bool
}

func (f *FormField) StringValue() string {
	switch f.Type {
	case FormFieldSelect, FormFieldToggle:
		if f.SelIdx >= 0 && f.SelIdx < len(f.Options) {
			return f.Options[f.SelIdx]
		}
	}
	return f.Value
}

func (f *FormField) BoolValue() bool {
	return f.SelIdx == 0
}

func TextField(label, defaultVal string, required bool) *FormField {
	return &FormField{Label: label, Type: FormFieldText, Value: defaultVal, Required: required}
}

func PasswordField(label string) *FormField {
	return &FormField{Label: label, Type: FormFieldPassword}
}

func SelectField(label string, options []string, defaultIdx int) *FormField {
	if defaultIdx < 0 || defaultIdx >= len(options) {
		defaultIdx = 0
	}
	return &FormField{Label: label, Type: FormFieldSelect, Options: options, SelIdx: defaultIdx}
}

func ToggleField(label string, defaultVal bool) *FormField {
	idx := 1
	if defaultVal {
		idx = 0
	}
	return &FormField{
		Label:   label,
		Type:    FormFieldToggle,
		Options: []string{"yes", "no"},
		SelIdx:  idx,
	}
}

func ShowForm(title string, fields []*FormField) (bool, error) {
	raw, err := EnterRawMode()
	if err != nil {
		return false, err
	}
	defer raw.Restore()

	HideCursor()

	inputs := make([][]rune, len(fields))
	cursors := make([]int, len(fields))
	invalid := make([]bool, len(fields))
	for i, f := range fields {
		inputs[i] = []rune(f.Value)
		cursors[i] = len(inputs[i])
	}

	isVisible := func(i int) bool {
		if i < 0 || i >= len(fields) {
			return false
		}
		if fields[i].VisibleWhen == nil {
			return true
		}
		return fields[i].VisibleWhen(fields)
	}

	focus := 0
	for focus < len(fields) && !isVisible(focus) {
		focus++
	}
	if focus >= len(fields) {
		focus = 0
	}

	maxLabel := 0
	for _, f := range fields {
		if l := len([]rune(f.Label)); l > maxLabel {
			maxLabel = l
		}
	}
	labelW := maxLabel + 2

	render := func() int {
		lines := 0
		Writeln("")
		lines++
		Writeln("  " + pterm.Bold.Sprint(title))
		lines++
		Writeln("  " + pterm.FgGray.Sprint(strings.Repeat("─", 56)))
		lines++
		Writeln("")
		lines++

		for i, f := range fields {
			if !isVisible(i) {
				continue
			}
			isFocus := i == focus
			isInvalid := invalid[i]

			var pfx string
			if isFocus {
				pfx = "  " + pterm.FgCyan.Sprint("❯") + " "
			} else {
				pfx = "    "
			}

			lbl := fmt.Sprintf("%-*s", labelW, f.Label)
			switch {
			case isInvalid && !isFocus:
				lbl = pterm.FgRed.Sprint(lbl)
			case isFocus:
				lbl = pterm.FgWhite.Sprint(lbl)
			default:
				lbl = pterm.FgGray.Sprint(lbl)
			}

			var val string
			switch f.Type {
			case FormFieldText, FormFieldPassword:
				inp := inputs[i]
				cur := cursors[i]
				if isFocus {
					var sb strings.Builder
					for j, ch := range inp {
						if j == cur {
							sb.WriteString("\033[7m")
						}
						if f.Type == FormFieldPassword {
							sb.WriteRune('●')
						} else {
							sb.WriteRune(ch)
						}
						if j == cur {
							sb.WriteString("\033[27m")
						}
					}
					if cur >= len(inp) {
						sb.WriteString("\033[7m \033[27m")
					}
					val = sb.String()
				} else {
					if len(inp) == 0 {
						switch {
						case isInvalid:
							val = pterm.FgRed.Sprint("(required)")
						case f.Required:
							val = pterm.FgGray.Sprint("(required)")
						case f.Placeholder != "":
							val = pterm.FgGray.Sprint(f.Placeholder)
						default:
							val = pterm.FgGray.Sprint("—")
						}
					} else if f.Type == FormFieldPassword {
						val = pterm.FgGray.Sprint(strings.Repeat("●", len(inp)))
					} else {
						val = pterm.FgGray.Sprint(string(inp))
					}
				}

			case FormFieldSelect, FormFieldToggle:
				opt := f.Options[f.SelIdx]
				if isFocus {
					var l, r string
					if f.SelIdx > 0 {
						l = pterm.FgCyan.Sprint("‹") + " "
					}
					if f.SelIdx < len(f.Options)-1 {
						r = " " + pterm.FgCyan.Sprint("›")
					}
					val = l + pterm.FgWhite.Sprint(opt) + r
				} else {
					val = pterm.FgGray.Sprint(opt)
				}
			}

			Writeln(pfx + lbl + " " + val)
			lines++
		}

		Writeln("")
		lines++
		Write("  " + pterm.FgGray.Sprint(
			"Tab/Shift+Tab  navigate  ·  ←/→  cycle/cursor  ·  Enter  next  ·  Ctrl+S  submit  ·  Esc  cancel",
		))
		lines++
		return lines
	}

	updateFn := func(fieldIdx int, val string) {
		if fieldIdx >= 0 && fieldIdx < len(fields) {
			inputs[fieldIdx] = []rune(val)
			cursors[fieldIdx] = len(inputs[fieldIdx])
		}
	}

	trySubmit := func() bool {
		for i, f := range fields {
			if !isVisible(i) {
				continue
			}
			if f.Type == FormFieldText || f.Type == FormFieldPassword {
				f.Value = strings.TrimSpace(string(inputs[i]))
			}
		}
		ok := true
		for i, f := range fields {
			invalid[i] = false
			if !isVisible(i) {
				continue
			}
			if f.Required && (f.Type == FormFieldText || f.Type == FormFieldPassword) {
				if f.Value == "" {
					invalid[i] = true
					ok = false
				}
			}
		}
		if !ok {
			for i, inv := range invalid {
				if inv && isVisible(i) {
					focus = i
					break
				}
			}
		}
		return ok
	}

	renderedLines := render()
	buf := make([]byte, 64)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			ExitClean(renderedLines)
			return false, err
		}
		if n == 0 {
			continue
		}

		f := fields[focus]

		if n >= 3 && buf[0] == 27 && buf[1] == '[' {
			switch buf[2] {
			case 'A': // Up
				for prev := focus - 1; prev >= 0; prev-- {
					if isVisible(prev) {
						focus = prev
						break
					}
				}
			case 'B': // Down
				for next := focus + 1; next < len(fields); next++ {
					if isVisible(next) {
						focus = next
						break
					}
				}
			case 'C': // Right
				if f.Type == FormFieldSelect || f.Type == FormFieldToggle {
					if f.SelIdx < len(f.Options)-1 {
						f.SelIdx++
						if f.OnChange != nil {
							f.OnChange(f.SelIdx, inputs, updateFn)
						}
					}
				} else if cursors[focus] < len(inputs[focus]) {
					cursors[focus]++
				}
			case 'D': // Left
				if f.Type == FormFieldSelect || f.Type == FormFieldToggle {
					if f.SelIdx > 0 {
						f.SelIdx--
						if f.OnChange != nil {
							f.OnChange(f.SelIdx, inputs, updateFn)
						}
					}
				} else if cursors[focus] > 0 {
					cursors[focus]--
				}
			case 'Z': // Shift+Tab
				for prev := focus - 1; prev >= 0; prev-- {
					if isVisible(prev) {
						focus = prev
						break
					}
				}
			case '3': // Delete
				if n >= 4 && buf[3] == '~' {
					if (f.Type == FormFieldText || f.Type == FormFieldPassword) && cursors[focus] < len(inputs[focus]) {
						inp := inputs[focus]
						inputs[focus] = append(inp[:cursors[focus]], inp[cursors[focus]+1:]...)
						invalid[focus] = false
					}
				}
			case 'H': // Home
				if f.Type == FormFieldText || f.Type == FormFieldPassword {
					cursors[focus] = 0
				}
			case 'F': // End
				if f.Type == FormFieldText || f.Type == FormFieldPassword {
					cursors[focus] = len(inputs[focus])
				}
			}
		} else if n >= 3 && buf[0] == 27 && buf[1] == 'O' {
			switch buf[2] {
			case 'H':
				cursors[focus] = 0
			case 'F':
				cursors[focus] = len(inputs[focus])
			}
		} else if n == 1 {
			switch buf[0] {
			case 3, 27: // Ctrl+C or Esc
				ExitClean(renderedLines)
				return false, nil
			case 9: // Tab
				for next := focus + 1; next < len(fields); next++ {
					if isVisible(next) {
						focus = next
						break
					}
				}
			case 13: // Enter
				next := -1
				for nn := focus + 1; nn < len(fields); nn++ {
					if isVisible(nn) {
						next = nn
						break
					}
				}
				if next != -1 {
					focus = next
				} else {
					if trySubmit() {
						ExitClean(renderedLines)
						return true, nil
					}
				}
			case 127, 8: // Backspace
				if (f.Type == FormFieldText || f.Type == FormFieldPassword) && cursors[focus] > 0 {
					inp := inputs[focus]
					inputs[focus] = append(inp[:cursors[focus]-1], inp[cursors[focus]:]...)
					cursors[focus]--
					invalid[focus] = false
				}
			case 19: // Ctrl+S
				if trySubmit() {
					ExitClean(renderedLines)
					return true, nil
				}
			default:
				if buf[0] >= 32 && (f.Type == FormFieldText || f.Type == FormFieldPassword) {
					ch := rune(buf[0])
					inp := inputs[focus]
					newInp := make([]rune, len(inp)+1)
					copy(newInp, inp[:cursors[focus]])
					newInp[cursors[focus]] = ch
					copy(newInp[cursors[focus]+1:], inp[cursors[focus]:])
					inputs[focus] = newInp
					cursors[focus]++
					invalid[focus] = false
				}
			}
		} else if n > 1 && buf[0] != 27 {
			if f.Type == FormFieldText || f.Type == FormFieldPassword {
				runes := []rune(string(buf[:n]))
				cur := cursors[focus]
				inp := inputs[focus]
				var printable []rune
				for _, r := range runes {
					if r >= 32 && r != 127 {
						printable = append(printable, r)
					}
				}
				if len(printable) > 0 {
					newInp := make([]rune, len(inp)+len(printable))
					copy(newInp, inp[:cur])
					copy(newInp[cur:], printable)
					copy(newInp[cur+len(printable):], inp[cur:])
					inputs[focus] = newInp
					cursors[focus] += len(printable)
					invalid[focus] = false
				}
			}
		}

		ClearLines(renderedLines)
		renderedLines = render()
	}
}
