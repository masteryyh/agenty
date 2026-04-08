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
	"unicode/utf8"

	"github.com/pterm/pterm"
)

func ReadLine(prompt string, masked bool) (string, error) {
	raw, err := EnterRawMode()
	if err != nil {
		return "", err
	}
	defer raw.Restore()

	ShowCursor()
	Write("  " + pterm.FgYellow.Sprint("?") + " " + pterm.Bold.Sprint(prompt) + ": ")

	var input []rune
	buf := make([]byte, 64)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			Writeln("")
			return "", err
		}

		if n >= 2 && buf[0] == 27 {
			continue
		}

		var done bool
		for i := 0; i < n; {
			b := buf[i]
			switch {
			case b == 13:
				if len(input) == 0 {
					i++
					continue
				}
				done = true
				i++
			case b == 27 || b == 3:
				Writeln("")
				return "", ErrCancelled
			case b == 127 || b == 8:
				if len(input) > 0 {
					input = input[:len(input)-1]
					Write("\b \b")
				}
				i++
			case b >= 32:
				r, size := utf8.DecodeRune(buf[i:n])
				if r == utf8.RuneError && size <= 1 {
					i++
					continue
				}
				input = append(input, r)
				if masked {
					Write("*")
				} else {
					Write(string(r))
				}
				i += size
			default:
				i++
			}
			if done {
				break
			}
		}
		if done {
			Writeln("")
			return strings.TrimSpace(string(input)), nil
		}
	}
}

func ReadText(prompt string) (string, error) {
	return ReadLine(prompt, false)
}

func ReadPassword(prompt string) (string, error) {
	return ReadLine(prompt, true)
}
