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
	"errors"
	"fmt"
	"os"

	"golang.org/x/term"
)

var ErrCancelled = errors.New("user cancelled")

type RawMode struct {
	fd       int
	oldState *term.State
}

func EnterRawMode() (*RawMode, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("failed to set raw terminal: %w", err)
	}
	return &RawMode{fd: fd, oldState: oldState}, nil
}

func (r *RawMode) Restore() {
	term.Restore(r.fd, r.oldState)
}

func Write(s string) {
	os.Stdout.WriteString(s)
}

func Writeln(s string) {
	os.Stdout.WriteString(s + "\r\n")
}

func HideCursor() {
	Write("\033[?25l")
}

func ShowCursor() {
	Write("\033[?25h")
}

func ClearCurrentLine() {
	Write("\033[2K")
}

func ClearLines(n int) {
	for i := 0; i < n-1; i++ {
		Write("\033[A")
	}
	Write("\r")
	for i := range n {
		Write("\033[2K")
		if i < n-1 {
			Write("\033[B")
		}
	}
	for i := 0; i < n-1; i++ {
		Write("\033[A")
	}
	Write("\r")
}

func ExitClean(renderedLines int) {
	ClearLines(renderedLines)
	ShowCursor()
}
