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
	"math/rand/v2"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/masteryyh/agenty/pkg/providers"
	"github.com/muesli/reflow/ansi"
	"github.com/muesli/reflow/wordwrap"
	"github.com/muesli/reflow/wrap"
)

type streamModel struct {
	active         bool
	headerPrinted  bool
	hasReasoning   bool
	hasToolSection bool
	hadToolCalls   bool
	hasContent     bool
	atLineStart    bool

	buf            *strings.Builder
	reasoningBuf   *strings.Builder
	reasoningStart time.Time
	inReasoning    bool
	sectionPrefix  string
	inContent      bool
	contentBuf     *strings.Builder

	ch     chan providers.StreamEvent
	doneCh chan error

	spinIdx   int
	tickCount int
	phrase    string
}

func newStreamModel() streamModel {
	return streamModel{
		buf:          new(strings.Builder),
		reasoningBuf: new(strings.Builder),
		contentBuf:   new(strings.Builder),
		atLineStart:  true,
	}
}

func (s *streamModel) start() {
	s.active = true
	s.spinIdx = 0
	s.tickCount = 0
	s.phrase = streamingPhrases[rand.IntN(len(streamingPhrases))]
	s.buf.Reset()
	s.headerPrinted = false
	s.hasReasoning = false
	s.hasToolSection = false
	s.hadToolCalls = false
	s.hasContent = false
	s.atLineStart = true
	s.reasoningBuf.Reset()
	s.contentBuf.Reset()
	s.inReasoning = false
	s.inContent = false
	s.sectionPrefix = ""
	s.reasoningStart = time.Time{}
	s.ch = make(chan providers.StreamEvent, 32)
	s.doneCh = make(chan error, 1)
}

func (s *streamModel) waitForEvent() tea.Cmd {
	ch := s.ch
	doneCh := s.doneCh
	return func() tea.Msg {
		evt, ok := <-ch
		if !ok {
			err := <-doneCh
			return streamDoneMsg{err: err}
		}
		return streamEventMsg{event: evt}
	}
}

func (s *streamModel) handleEvent(evt providers.StreamEvent, modelName string) {
	switch evt.Type {
	case providers.EventReasoningDelta:
		if !s.headerPrinted {
			s.buf.WriteString(renderAssistantHeader(modelName, time.Now()))
			s.buf.WriteString("\n")
			s.headerPrinted = true
		}
		if !s.hasReasoning {
			s.hasReasoning = true
			s.reasoningStart = time.Now()
			s.inReasoning = true
			s.sectionPrefix = s.buf.String()
		}
		s.reasoningBuf.WriteString(evt.Reasoning)

	case providers.EventContentDelta:
		if s.inReasoning {
			s.finalizeReasoning(true)
		}
		if !s.headerPrinted {
			s.buf.WriteString(renderAssistantHeader(modelName, time.Now()))
			s.buf.WriteString("\n")
			s.headerPrinted = true
		}
		if !s.inContent {
			if !s.hasContent && s.hadToolCalls {
				s.buf.WriteString(streamRenderFinalLabel())
			}
			s.hasContent = true
			s.sectionPrefix = s.buf.String()
			s.inContent = true
		}
		s.contentBuf.WriteString(evt.Content)

	case providers.EventToolCallStart:
		s.finalizeReasoning(true)
		s.finalizeContent()
		if !s.headerPrinted {
			s.buf.WriteString(renderAssistantHeader(modelName, time.Now()))
			s.buf.WriteString("\n")
			s.headerPrinted = true
		}
		if !s.hasToolSection {
			if s.hasContent || s.hasReasoning {
				s.buf.WriteString("\n")
			}
			s.buf.WriteString(streamRenderToolLabel())
			s.hasToolSection = true
			s.hadToolCalls = true
		}
		s.atLineStart = true

	case providers.EventToolCallDelta:

	case providers.EventToolCallDone:
		if evt.ToolCall != nil {
			s.buf.WriteString(streamRenderBuiltinToolCallLine(evt.ToolCall.Name, evt.ToolCall.Arguments))
			s.buf.WriteString("\n")
			s.atLineStart = true
		}

	case providers.EventToolResult:
		if evt.ToolResult != nil {
			if evt.ToolResult.IsError {
				s.buf.WriteString(streamRenderToolError())
			} else {
				s.buf.WriteString(streamRenderToolSuccess())
			}
			lines, moreCount := renderBuiltinToolResultLines(evt.ToolResult.Name, stripCR(evt.ToolResult.Content), maxToolResultLines)
			wrapW := renderWidth - 10
			if wrapW < 20 {
				wrapW = 20
			}
			for _, l := range lines {
				for _, wl := range strings.Split(wordwrap.String(l, wrapW), "\n") {
					if ansi.PrintableRuneWidth(wl) > wrapW {
						for _, hw := range strings.Split(wrap.String(wl, wrapW), "\n") {
							s.buf.WriteString(contentIndent + "    " + styleToolResult.Render(hw) + "\n")
						}
					} else {
						s.buf.WriteString(contentIndent + "    " + styleToolResult.Render(wl) + "\n")
					}
				}
			}
			if moreCount > 0 {
				s.buf.WriteString(contentIndent + "    " + styleGray.Render(fmt.Sprintf("...(%d more results)", moreCount)) + "\n")
			}
			s.atLineStart = true
		}

	case providers.EventMessageDone:
		s.finalizeReasoning(true)
		s.finalizeContent()
		if !s.atLineStart {
			s.buf.WriteString("\n")
		}
		s.hasReasoning = false
		s.hasToolSection = false
		s.hasContent = false
		s.atLineStart = true

	case providers.EventError:
		if !s.atLineStart {
			s.buf.WriteString("\n")
		}
		s.buf.WriteString(styleSysErr.Render(fmt.Sprintf("  Error: %s\n", evt.Error)))
	}
}

func (s *streamModel) finalizeReasoning(showReasoning bool) {
	if !s.inReasoning {
		return
	}
	dur := time.Since(s.reasoningStart)
	label := contentIndent + styleReasoningLabel.Render(fmt.Sprintf("thinking: (%.1fs)", dur.Seconds())) + "\n"
	s.buf.Reset()
	s.buf.WriteString(s.sectionPrefix)
	s.buf.WriteString(label)
	if showReasoning && s.reasoningBuf.Len() > 0 {
		s.buf.WriteString(renderReasoningContent(s.reasoningBuf.String()))
	}
	s.buf.WriteString("\n")
	s.inReasoning = false
	s.sectionPrefix = ""
	s.reasoningBuf.Reset()
	s.atLineStart = true
}

func (s *streamModel) finalizeContent() {
	if !s.inContent {
		return
	}
	rendered := renderContentBlock(s.contentBuf.String())
	s.buf.Reset()
	s.buf.WriteString(s.sectionPrefix)
	s.buf.WriteString(rendered)
	s.inContent = false
	s.sectionPrefix = ""
	s.contentBuf.Reset()
	s.atLineStart = true
}

func (s *streamModel) finalize(showReasoning bool) string {
	s.finalizeReasoning(showReasoning)
	s.finalizeContent()
	if s.buf.Len() > 0 {
		result := s.buf.String() + "\n\n"
		s.buf.Reset()
		return result
	}
	return ""
}

func (s *streamModel) liveContent(showReasoning bool) string {
	if s.inReasoning {
		elapsed := time.Since(s.reasoningStart)
		label := contentIndent + styleReasoningLabel.Render(fmt.Sprintf("thinking: (%.1fs)", elapsed.Seconds())) + "\n"
		part := s.sectionPrefix + label
		if showReasoning && s.reasoningBuf.Len() > 0 {
			part += renderReasoningContent(s.reasoningBuf.String())
		}
		return part
	}
	if s.inContent {
		rendered := renderContentBlock(s.contentBuf.String())
		return s.sectionPrefix + rendered
	}
	return s.buf.String()
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var streamingPhrases = []string{
	"Brainstorming...",
	"Thundering...",
	"Processing...",
	"Connecting dots...",
	"Exploring ideas...",
	"Crafting a response...",
	"Pondering...",
	"Working on it...",
	"Consulting the oracle...",
	"Firing synapses...",
	"Chewing on that...",
	"Hmm...",
	"Reticulating splines...",
}

func streamRenderToolLabel() string {
	return contentIndent + styleToolLabel.Render("🔧 tool execution:") + "\n"
}

func streamRenderToolSuccess() string {
	return contentIndent + "  " + styleToolSuccess.Render("✓ ok") + "\n"
}

func streamRenderToolError() string {
	return contentIndent + "  " + styleToolError.Render("✗ error") + "\n"
}

func streamRenderFinalLabel() string {
	return "\n" + contentIndent + styleFinalLabel.Render("📝 final response:") + "\n"
}
