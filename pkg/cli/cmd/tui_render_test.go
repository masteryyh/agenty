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
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/providers"
	"github.com/muesli/termenv"
)

func TestStreamRenderOmitsFinalResponseLabelAfterToolCalls(t *testing.T) {
	stream := newStreamModel()
	stream.handleEvent(providers.StreamEvent{Type: providers.EventToolCallStart}, "test-model")
	stream.handleEvent(providers.StreamEvent{
		Type: providers.EventToolCallDone,
		ToolCall: &models.ToolCall{
			ID:        "call_123456",
			Name:      "read_file",
			Arguments: "{}",
		},
	}, "test-model")
	stream.handleEvent(providers.StreamEvent{
		Type: providers.EventToolResult,
		ToolResult: &models.ToolResult{
			CallID:  "call_123456",
			Name:    "read_file",
			Content: "ok",
		},
	}, "test-model")
	stream.handleEvent(providers.StreamEvent{
		Type:    providers.EventContentDelta,
		Content: "done",
	}, "test-model")

	got := stream.render(false)
	if strings.Contains(got, "final response") {
		t.Fatalf("stream render contains final response label: %q", got)
	}
	if !strings.Contains(got, "done") {
		t.Fatalf("stream render missing assistant content: %q", got)
	}
}

func TestHistoryRenderOmitsFinalResponseLabelAfterToolCalls(t *testing.T) {
	assistant := &models.ChatMessageDto{
		Role: models.RoleAssistant,
		ToolCalls: []models.ToolCall{{
			ID:        "call_123456",
			Name:      "read_file",
			Arguments: "{}",
		}},
	}
	finalResponse := &models.ChatMessageDto{
		Role:    models.RoleAssistant,
		Content: "done",
	}

	got := renderToolCallingSequence(assistant, nil, finalResponse, false)
	if strings.Contains(got, "final response") {
		t.Fatalf("history render contains final response label: %q", got)
	}
	if !strings.Contains(got, "done") {
		t.Fatalf("history render missing assistant content: %q", got)
	}
}

func TestRenderHintsLineShowsClipboardNoticeTemporarily(t *testing.T) {
	m := chatModel{
		width:                80,
		help:                 newHelpModel(),
		tokenConsumed:        42,
		clipboardNoticeUntil: time.Now().Add(time.Second),
	}

	got := xansi.Strip(m.renderHintsLine())
	if !strings.Contains(got, "copied to clipboard") {
		t.Fatalf("hints line missing clipboard notice: %q", got)
	}

	m.clipboardNoticeUntil = time.Now().Add(-time.Second)
	got = xansi.Strip(m.renderHintsLine())
	if strings.Contains(got, "copied to clipboard") {
		t.Fatalf("expired clipboard notice is still visible: %q", got)
	}
}

func TestSelectedTextFromHistoryLines(t *testing.T) {
	lines := []string{
		"  hello world   ",
		"  second line   ",
	}
	selection := mouseSelection{
		region: selectableRegionHistory,
		start:  selectionPoint{line: 0, col: 2},
		end:    selectionPoint{line: 1, col: 7},
	}

	got := selectedTextFromLines(lines, selection, 0)
	want := "hello world\n  second"
	if got != want {
		t.Fatalf("selected text mismatch:\nwant %q\ngot  %q", want, got)
	}
}

func TestSelectedTextFromInputLinesSkipsPrompt(t *testing.T) {
	lines := []string{
		"  prompt text   ",
		"  next line     ",
	}
	selection := mouseSelection{
		region: selectableRegionInput,
		start:  selectionPoint{line: 0, col: 0},
		end:    selectionPoint{line: 1, col: 6},
	}

	got := selectedTextFromLines(lines, selection, 2)
	want := "prompt text\nnext"
	if got != want {
		t.Fatalf("selected input text mismatch:\nwant %q\ngot  %q", want, got)
	}
}

func TestRenderReasoningAndContentStylesDiffer(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)

	reasoning := renderReasoningContent("thinking text")
	if !containsSGRParam(reasoning, "3") {
		t.Fatalf("reasoning render is not italic: %q", reasoning)
	}

	plainContent := renderContentBlock("answer text")
	if containsSGRParam(plainContent, "3") {
		t.Fatalf("plain assistant content render is italic: %q", plainContent)
	}
	if !strings.Contains(xansi.Strip(plainContent), "answer text") {
		t.Fatalf("plain assistant content missing text: %q", plainContent)
	}
	if !containsSGRParam(plainContent, "38") {
		t.Fatalf("plain assistant content render missing foreground color: %q", plainContent)
	}
	if strings.Contains(plainContent, "38;5;244") {
		t.Fatalf("plain assistant content uses reasoning color: %q", plainContent)
	}

	italicContent := renderContentBlock("*answer text*")
	if !containsSGRParam(italicContent, "3") {
		t.Fatalf("markdown italic was not preserved: %q", italicContent)
	}
	if strings.Contains(italicContent, "38;5;244") {
		t.Fatalf("italic assistant content uses reasoning color: %q", italicContent)
	}

	boldContent := renderContentBlock("**answer text**")
	if !containsSGRParam(boldContent, "1") {
		t.Fatalf("markdown bold was not preserved: %q", boldContent)
	}
}

func containsSGRParam(s, param string) bool {
	for {
		start := strings.Index(s, "\x1b[")
		if start == -1 {
			return false
		}
		s = s[start+2:]
		end := strings.IndexByte(s, 'm')
		if end == -1 {
			return false
		}
		for part := range strings.SplitSeq(s[:end], ";") {
			if part == param {
				return true
			}
		}
		s = s[end+1:]
	}
}
