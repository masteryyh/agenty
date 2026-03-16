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
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pterm/pterm"
	"golang.org/x/term"
)

var ErrCancelled = errors.New("user cancelled")

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

func rawWrite(s string) {
	os.Stdout.WriteString(s)
}

func rawWriteln(s string) {
	os.Stdout.WriteString(s + "\r\n")
}

func showInteractiveList(title string, items []string, hints string) (*ListResult, error) {
	if len(items) == 0 {
		return &ListResult{Action: ListActionCancel, Index: -1}, nil
	}

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("failed to set raw terminal: %w", err)
	}
	defer term.Restore(fd, oldState)

	cursor := 0
	maxVisible := 10
	offset := 0

	render := func() int {
		lines := 0

		rawWrite("\033[?25l")

		rawWriteln("  " + pterm.Bold.Sprint(title))
		lines++
		rawWriteln("  " + pterm.FgGray.Sprint(strings.Repeat("─", 56)))
		lines++
		rawWriteln("")
		lines++

		visibleEnd := min(offset + maxVisible, len(items))

		for i := offset; i < visibleEnd; i++ {
			if i == cursor {
				rawWriteln("  " + pterm.FgCyan.Sprint("❯") + " " + pterm.FgWhite.Sprint(items[i]))
			} else {
				rawWriteln("    " + pterm.FgGray.Sprint(items[i]))
			}
			lines++
		}

		if len(items) > maxVisible {
			rawWriteln("  " + pterm.FgGray.Sprintf("(%d/%d)", cursor+1, len(items)))
			lines++
		}

		rawWriteln("")
		lines++
		rawWrite("  " + pterm.FgGray.Sprint(hints))
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

	exitClean := func(renderedLines int) {
		clearLines(renderedLines)
		rawWrite("\033[?25h")
	}

	renderedLines := render()

	buf := make([]byte, 64)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			exitClean(renderedLines)
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
				clearLines(renderedLines)
				renderedLines = render()
				continue
			case 66: // Down
				if cursor < len(items)-1 {
					cursor++
					if cursor >= offset+maxVisible {
						offset = cursor - maxVisible + 1
					}
				}
				clearLines(renderedLines)
				renderedLines = render()
				continue
			}
		}

		if n == 1 {
			switch buf[0] {
			case 27: // Esc
				exitClean(renderedLines)
				return &ListResult{Action: ListActionCancel, Index: -1}, nil
			case 13: // Enter
				exitClean(renderedLines)
				return &ListResult{Action: ListActionSelect, Index: cursor}, nil
			case 'a', 'A':
				exitClean(renderedLines)
				return &ListResult{Action: ListActionAdd, Index: cursor}, nil
			case 'e', 'E':
				exitClean(renderedLines)
				return &ListResult{Action: ListActionEdit, Index: cursor}, nil
			case 4: // Ctrl+D
				exitClean(renderedLines)
				return &ListResult{Action: ListActionDelete, Index: cursor}, nil
			}
		}
	}
}

func showConfirm(message string) (bool, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return false, fmt.Errorf("failed to set raw terminal: %w", err)
	}
	defer term.Restore(fd, oldState)

	rawWrite("\033[2K\r  " + pterm.FgYellow.Sprint("?") + " " + message + " " + pterm.FgGray.Sprint("[y/N] "))

	buf := make([]byte, 64)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return false, err
		}
		if n < 1 {
			continue
		}

		switch buf[0] {
		case 'y', 'Y':
			rawWriteln(pterm.FgGreen.Sprint("Yes"))
			return true, nil
		case 'n', 'N', 13, 27:
			rawWriteln(pterm.FgRed.Sprint("No"))
			return false, nil
		}
	}
}

func readInput(prompt, defaultValue string) (string, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", fmt.Errorf("failed to set raw terminal: %w", err)
	}
	defer term.Restore(fd, oldState)

	rawWrite("\033[?25h")
	rawWrite("  " + pterm.FgYellow.Sprint("?") + " " + pterm.Bold.Sprint(prompt))
	if defaultValue != "" {
		rawWrite(" [" + pterm.FgGray.Sprint(defaultValue) + "]: ")
	} else {
		rawWrite(": ")
	}

	input := []rune(defaultValue)
	for _, ch := range input {
		rawWrite(string(ch))
	}

	buf := make([]byte, 64)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			rawWriteln("")
			return "", err
		}

		if n >= 3 && buf[0] == 27 && buf[1] == 91 {
			continue
		}

		if n == 1 {
			switch buf[0] {
			case 13:
				rawWriteln("")
				result := strings.TrimSpace(string(input))
				if result == "" {
					return defaultValue, nil
				}
				return result, nil
			case 27, 3:
				rawWriteln("")
				return "", ErrCancelled
			case 127, 8:
				if len(input) > 0 {
					input = input[:len(input)-1]
					rawWrite("\b \b")
				}
			default:
				if buf[0] >= 32 {
					input = append(input, rune(buf[0]))
					rawWrite(string(buf[0]))
				}
			}
		}
	}
}

func readPassword(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", fmt.Errorf("failed to set raw terminal: %w", err)
	}
	defer term.Restore(fd, oldState)

	rawWrite("\033[?25h")
	rawWrite("  " + pterm.FgYellow.Sprint("?") + " " + pterm.Bold.Sprint(prompt) + ": ")

	var input []rune
	buf := make([]byte, 64)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			rawWriteln("")
			return "", err
		}

		if n >= 3 && buf[0] == 27 && buf[1] == 91 {
			continue
		}

		if n == 1 {
			switch buf[0] {
			case 13:
				rawWriteln("")
				return strings.TrimSpace(string(input)), nil
			case 27, 3:
				rawWriteln("")
				return "", ErrCancelled
			case 127, 8:
				if len(input) > 0 {
					input = input[:len(input)-1]
					rawWrite("\b \b")
				}
			default:
				if buf[0] >= 32 {
					input = append(input, rune(buf[0]))
					rawWrite("*")
				}
			}
		}
	}
}

func selectOption(title string, options []string, defaultIndex int) (int, error) {
	if defaultIndex < 0 || defaultIndex >= len(options) {
		defaultIndex = 0
	}

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return -1, fmt.Errorf("failed to set raw terminal: %w", err)
	}
	defer term.Restore(fd, oldState)

	cursor := defaultIndex

	render := func() int {
		lines := 0
		rawWrite("\033[?25l")
		rawWriteln("  " + pterm.Bold.Sprint(title))
		lines++
		rawWriteln("  " + pterm.FgGray.Sprint(strings.Repeat("─", 40)))
		lines++
		rawWriteln("")
		lines++

		for i, opt := range options {
			if i == cursor {
				rawWriteln("  " + pterm.FgCyan.Sprint("❯") + " " + pterm.FgWhite.Sprint(opt))
			} else {
				rawWriteln("    " + pterm.FgGray.Sprint(opt))
			}
			lines++
		}

		rawWriteln("")
		lines++
		rawWrite("  " + pterm.FgGray.Sprint("↑/↓ navigate  ·  Enter select  ·  Esc back"))
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

	exitClean := func(renderedLines int) {
		clearLines(renderedLines)
		rawWrite("\033[?25h")
	}

	renderedLines := render()

	buf := make([]byte, 64)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			exitClean(renderedLines)
			return -1, err
		}

		if n >= 3 && buf[0] == 27 && buf[1] == 91 {
			switch buf[2] {
			case 65:
				if cursor > 0 {
					cursor--
				}
				clearLines(renderedLines)
				renderedLines = render()
				continue
			case 66:
				if cursor < len(options)-1 {
					cursor++
				}
				clearLines(renderedLines)
				renderedLines = render()
				continue
			}
		}

		if n == 1 {
			switch buf[0] {
			case 13:
				exitClean(renderedLines)
				return cursor, nil
			case 27:
				exitClean(renderedLines)
				return -1, ErrCancelled
			}
		}
	}
}

