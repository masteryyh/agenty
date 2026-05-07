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

	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/providers"
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
