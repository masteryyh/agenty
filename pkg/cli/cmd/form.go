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
	"os"
	"strings"

	"github.com/pterm/pterm"
	"golang.org/x/term"
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
	Placeholder string // shown when empty and unfocused (optional)
	// OnChange is called when SelIdx changes. inputs is the current input state
	// for all fields; update(fieldIdx, val) updates a text/password field's value.
	OnChange func(selIdx int, inputs [][]rune, update func(fieldIdx int, val string))
	// VisibleWhen, if set, is called before each render to decide if the field
	// should be shown. Hidden fields are skipped in rendering, navigation and
	// validation. When nil the field is always visible.
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

// ShowForm renders an interactive form with the given title and fields. Returns
// (true, nil) when submitted, (false, nil) when cancelled via Esc.
func ShowForm(title string, fields []*FormField) (bool, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return false, fmt.Errorf("failed to set raw terminal: %w", err)
	}
	defer term.Restore(fd, oldState)

	rawWrite("\033[?25l")

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
		rawWriteln("")
		lines++
		rawWriteln("  " + pterm.Bold.Sprint(title))
		lines++
		rawWriteln("  " + pterm.FgGray.Sprint(strings.Repeat("─", 56)))
		lines++
		rawWriteln("")
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

			rawWriteln(pfx + lbl + " " + val)
			lines++
		}

		rawWriteln("")
		lines++
		rawWrite("  " + pterm.FgGray.Sprint(
			"Tab/Shift+Tab  navigate  ·  ←/→  cycle/cursor  ·  Enter  next  ·  Ctrl+S  submit  ·  Esc  cancel",
		))
		lines++
		return lines
	}

	clearLines := func(n int) {
		for i := 0; i < n-1; i++ {
			rawWrite("\033[A")
		}
		rawWrite("\r")
		for i := range n {
			rawWrite("\033[2K")
			if i < n-1 {
				rawWrite("\033[B")
			}
		}
		for i := 0; i < n-1; i++ {
			rawWrite("\033[A")
		}
		rawWrite("\r")
	}

	exitClean := func(n int) {
		clearLines(n)
		rawWrite("\033[?25h")
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
			exitClean(renderedLines)
			return false, err
		}
		if n == 0 {
			continue
		}

		f := fields[focus]

		if n >= 3 && buf[0] == 27 && buf[1] == '[' {
			switch buf[2] {
			case 'A': // Up → prev visible field
				for prev := focus - 1; prev >= 0; prev-- {
					if isVisible(prev) {
						focus = prev
						break
					}
				}
			case 'B': // Down → next visible field
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
			case 'Z': // Shift+Tab → prev visible field
				for prev := focus - 1; prev >= 0; prev-- {
					if isVisible(prev) {
						focus = prev
						break
					}
				}
			case '3': // Delete (ESC[3~)
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
			case 3, 27: // Ctrl+C or Esc → cancel
				exitClean(renderedLines)
				return false, nil
			case 9: // Tab → next visible field
				for next := focus + 1; next < len(fields); next++ {
					if isVisible(next) {
						focus = next
						break
					}
				}
			case 13: // Enter → next visible or submit
				next := -1
				for n := focus + 1; n < len(fields); n++ {
					if isVisible(n) {
						next = n
						break
					}
				}
				if next != -1 {
					focus = next
				} else {
					if trySubmit() {
						exitClean(renderedLines)
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
			case 19: // Ctrl+S → submit
				if trySubmit() {
					exitClean(renderedLines)
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
		}

		clearLines(renderedLines)
		renderedLines = render()
	}
}
