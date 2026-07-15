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

package termwrap

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/ansi"
	"github.com/muesli/reflow/wordwrap"
	"github.com/muesli/reflow/wrap"
)

type Options struct {
	Width                    int
	Indent                   string
	Style                    lipgloss.Style
	TrimLeadingVisibleSpaces int
}

func WrapLines(text string, opts Options) string {
	text = StripCR(text)
	wrapWidth := max(opts.Width, 20)
	var buf strings.Builder
	for line := range strings.SplitSeq(text, "\n") {
		if opts.TrimLeadingVisibleSpaces > 0 {
			line = TrimLeadingVisibleSpaces(line, opts.TrimLeadingVisibleSpaces)
		}
		for wl := range strings.SplitSeq(wordwrap.String(line, wrapWidth), "\n") {
			if ansi.PrintableRuneWidth(wl) > wrapWidth {
				for hw := range strings.SplitSeq(wrap.String(wl, wrapWidth), "\n") {
					buf.WriteString(opts.Indent)
					buf.WriteString(opts.Style.Render(hw))
					buf.WriteString("\n")
				}
			} else {
				buf.WriteString(opts.Indent)
				buf.WriteString(opts.Style.Render(wl))
				buf.WriteString("\n")
			}
		}
	}
	return buf.String()
}

func StripCR(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "")
}

func TrimLeadingVisibleSpaces(s string, n int) string {
	stripped := 0
	var kept strings.Builder
	i := 0
	for i < len(s) && stripped < n {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !(s[j] >= 0x40 && s[j] <= 0x7e) {
				j++
			}
			if j < len(s) {
				j++
			}
			kept.WriteString(s[i:j])
			i = j
		} else if s[i] == ' ' {
			stripped++
			i++
		} else {
			break
		}
	}
	return kept.String() + s[i:]
}
