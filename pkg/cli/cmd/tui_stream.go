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
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/providers"
)

type streamState int

const (
	streamIdle streamState = iota
	streamStreaming
	streamFinishing
	streamFailed
)

type streamSegmentKind int

const (
	streamSegmentHeader streamSegmentKind = iota
	streamSegmentReasoning
	streamSegmentContent
	streamSegmentToolLabel
	streamSegmentToolCall
	streamSegmentToolResult
	streamSegmentError
)

type streamModel struct {
	state         streamState
	showIndicator bool
	segments      []streamSegment
	toolCalls     map[string]streamToolCall

	currentReasoning int
	currentContent   int
	headerPrinted    bool
	hasToolSection   bool
	hasContent       bool

	ch     chan providers.StreamEvent
	doneCh chan error

	phrase string
}

type streamSegment struct {
	kind       streamSegmentKind
	modelName  string
	text       string
	toolCall   *models.ToolCall
	toolResult *models.ToolResult
	startedAt  time.Time
	endedAt    time.Time
}

type streamToolCall struct {
	name string
}

func newStreamModel() streamModel {
	return streamModel{
		state:            streamIdle,
		toolCalls:        make(map[string]streamToolCall),
		currentReasoning: -1,
		currentContent:   -1,
	}
}

func (s *streamModel) start() {
	s.state = streamStreaming
	s.showIndicator = true
	s.phrase = streamingPhrases[rand.IntN(len(streamingPhrases))]
	s.resetAttempt()
	s.ch = make(chan providers.StreamEvent, 32)
	s.doneCh = make(chan error, 1)
}

func (s *streamModel) resetAttempt() {
	s.segments = nil
	s.toolCalls = make(map[string]streamToolCall)
	s.currentReasoning = -1
	s.currentContent = -1
	s.headerPrinted = false
	s.hasToolSection = false
	s.hasContent = false
}

func (s *streamModel) waitForEvent() tea.Cmd {
	ch := s.ch
	doneCh := s.doneCh
	return func() tea.Msg {
		evt, ok := <-ch
		if !ok {
			select {
			case err := <-doneCh:
				return streamDoneMsg{err: err}
			default:
				return streamDoneMsg{}
			}
		}
		return streamEventMsg{event: evt}
	}
}

func (s *streamModel) handleEvent(evt providers.StreamEvent, modelName string) {
	switch evt.Type {
	case providers.EventReasoningDelta:
		s.ensureHeader(modelName)
		if s.currentReasoning == -1 {
			s.currentReasoning = len(s.segments)
			s.segments = append(s.segments, streamSegment{kind: streamSegmentReasoning, startedAt: time.Now()})
		}
		s.segments[s.currentReasoning].text += evt.Reasoning

	case providers.EventContentDelta:
		s.finishReasoning()
		s.ensureHeader(modelName)
		if s.currentContent == -1 {
			s.currentContent = len(s.segments)
			s.segments = append(s.segments, streamSegment{kind: streamSegmentContent})
			s.hasContent = true
		}
		s.segments[s.currentContent].text += evt.Content

	case providers.EventToolCallStart:
		s.finishReasoning()
		s.finishContent()
		s.ensureHeader(modelName)
		if !s.hasToolSection {
			s.segments = append(s.segments, streamSegment{kind: streamSegmentToolLabel})
			s.hasToolSection = true
		}

	case providers.EventToolCallDone:
		if evt.ToolCall != nil {
			s.rememberToolCall(*evt.ToolCall)
			tc := *evt.ToolCall
			s.segments = append(s.segments, streamSegment{kind: streamSegmentToolCall, toolCall: &tc})
		}

	case providers.EventToolResult:
		if evt.ToolResult != nil {
			tr := *evt.ToolResult
			if tr.Name == "" {
				if tc, ok := s.toolCalls[tr.CallID]; ok {
					tr.Name = tc.name
				}
			}
			s.segments = append(s.segments, streamSegment{kind: streamSegmentToolResult, toolResult: &tr})
		}

	case providers.EventMessageDone:
		s.finishReasoning()
		s.finishContent()
		s.hasToolSection = false
		s.hasContent = false

	case providers.EventCompactionStart:
		s.finishReasoning()
		s.finishContent()
		s.showIndicator = true
		s.phrase = "Compacting conversation..."

	case providers.EventCompactionDone:
		s.showIndicator = true
		s.phrase = s.streamingPhrase()

	case providers.EventError:
		s.state = streamFailed
		s.showIndicator = false
		s.resetAttempt()
		s.segments = append(s.segments, streamSegment{kind: streamSegmentError, text: evt.Error})
	}
}

func (s *streamModel) finish() {
	s.finishReasoning()
	s.finishContent()
	if s.state != streamFailed {
		s.state = streamFinishing
	}
	s.showIndicator = false
}

func (s *streamModel) close() {
	s.state = streamIdle
	s.showIndicator = false
	s.resetAttempt()
}

func (s streamModel) busy() bool {
	return s.state == streamStreaming
}

func (s streamModel) visible() bool {
	return len(s.segments) > 0
}

func (s streamModel) reasoningActive() bool {
	return s.currentReasoning >= 0
}

func (s *streamModel) ensureHeader(modelName string) {
	if s.headerPrinted {
		return
	}
	s.segments = append(s.segments, streamSegment{kind: streamSegmentHeader, modelName: modelName, startedAt: time.Now()})
	s.headerPrinted = true
}

func (s *streamModel) finishReasoning() {
	if s.currentReasoning == -1 {
		return
	}
	if s.segments[s.currentReasoning].endedAt.IsZero() {
		s.segments[s.currentReasoning].endedAt = time.Now()
	}
	s.currentReasoning = -1
}

func (s *streamModel) finishContent() {
	s.currentContent = -1
}

func (s *streamModel) rememberToolCall(tc models.ToolCall) {
	if tc.ID == "" {
		return
	}
	s.toolCalls[tc.ID] = streamToolCall{name: tc.Name}
}

func (s *streamModel) streamingPhrase() string {
	if s.phrase == "" || s.phrase == "Compacting conversation..." {
		return streamingPhrases[rand.IntN(len(streamingPhrases))]
	}
	return s.phrase
}

func (s *streamModel) finalize(showReasoning bool) string {
	s.finish()
	if len(s.segments) == 0 {
		return ""
	}
	result := s.render(showReasoning) + "\n\n"
	s.resetAttempt()
	return result
}

func (s *streamModel) liveContent(showReasoning bool) string {
	return s.render(showReasoning)
}

func (s *streamModel) render(showReasoning bool) string {
	var buf strings.Builder
	for _, segment := range s.segments {
		switch segment.kind {
		case streamSegmentHeader:
			buf.WriteString(renderAssistantHeader(segment.modelName, segment.startedAt))
			buf.WriteString("\n")
		case streamSegmentReasoning:
			buf.WriteString(renderStreamReasoning(segment, showReasoning))
		case streamSegmentContent:
			buf.WriteString(renderContentBlock(segment.text))
		case streamSegmentToolLabel:
			buf.WriteString(streamRenderToolLabel())
		case streamSegmentToolCall:
			if segment.toolCall != nil {
				buf.WriteString(streamRenderBuiltinToolCallLine(segment.toolCall.Name, segment.toolCall.Arguments, segment.toolCall.ID))
				buf.WriteString("\n")
			}
		case streamSegmentToolResult:
			if segment.toolResult != nil {
				buf.WriteString(renderStreamToolResult(segment.toolResult))
			}
		case streamSegmentError:
			buf.WriteString(styleSysErr.Render(fmt.Sprintf("  Error: %s\n", segment.text)))
		}
	}
	return buf.String()
}

func renderStreamReasoning(segment streamSegment, show bool) string {
	end := segment.endedAt
	if end.IsZero() {
		end = time.Now()
	}
	duration := end.Sub(segment.startedAt)
	if duration < 0 {
		duration = 0
	}
	var buf strings.Builder
	buf.WriteString(contentIndent)
	buf.WriteString(styleReasoningLabel.Render(formatReasoningLabel(duration)))
	buf.WriteString("\n")
	if show && segment.text != "" {
		buf.WriteString(renderReasoningContent(segment.text))
	}
	buf.WriteString("\n")
	return buf.String()
}

func renderStreamToolResult(result *models.ToolResult) string {
	var buf strings.Builder
	buf.WriteString(renderToolResultHeader(result))
	lines, moreCount := renderBuiltinToolResultLines(result.Name, stripCR(result.Content), maxToolResultLines)
	wrapW := max(renderWidth-10, 20)
	for _, line := range lines {
		buf.WriteString(renderWrappedLines(line, wrapOptions{
			Width:  wrapW,
			Indent: contentIndent + "    ",
			Style:  styleToolResult,
		}))
	}
	if moreCount > 0 {
		buf.WriteString(contentIndent)
		buf.WriteString("    ")
		buf.WriteString(styleGray.Render(fmt.Sprintf("...(%d more results)", moreCount)))
		buf.WriteString("\n")
	}
	return buf.String()
}

func renderToolResultHeader(result *models.ToolResult) string {
	status := styleToolSuccess.Render("✓ ok")
	if result.IsError {
		status = styleToolError.Render("✗ error")
	}
	name := result.Name
	if name == "" {
		name = "tool"
	}
	id := shortToolCallID(result.CallID)
	if id != "" {
		return contentIndent + "  " + status + " " + styleGray.Render("for ") + styleToolName.Render(name) + styleGray.Render(" ["+id+"]") + "\n"
	}
	return contentIndent + "  " + status + " " + styleGray.Render("for ") + styleToolName.Render(name) + "\n"
}

var streamingPhrases = []string{
	"Brainstorming...",
	"Thundering...",
	"Processing...",
	"Connecting dots...",
	"Exploring ideas...",
	"Crafting a response...",
	"Pondering...",
	"Working on it...",
	"Discombobulating...",
	"Firing synapses...",
	"Cooking up something good...",
	"Deciphering...",
	"Seasoning...",
	"Precolating...",
	"Flibbertigibbeting...",
}

func streamRenderToolLabel() string {
	return contentIndent + styleToolLabel.Render("🔧 tool execution:") + "\n"
}
