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

package providers

import (
	"strings"
	"testing"
	"unicode/utf8"

	json "github.com/bytedance/sonic"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/tools"
)

func TestDeepSeekBuildRequestThinkingAndReasoningContent(t *testing.T) {
	provider := NewDeepSeekProvider()

	req := provider.buildRequest(&ChatRequest{
		Model:         "deepseek-v4-pro",
		Thinking:      true,
		ThinkingLevel: "xhigh",
		Messages: []Message{
			{Role: models.RoleUser, Content: "first"},
			{
				Role:             models.RoleAssistant,
				Content:          "",
				ReasoningContent: "need a tool",
				ToolCalls: []models.ToolCall{
					{ID: "call_1", Name: "lookup", Arguments: `{"q":"x"}`},
				},
			},
			{
				Role:       models.RoleTool,
				ToolResult: &models.ToolResult{CallID: "call_1", Name: "lookup", Content: "result"},
			},
		},
		Tools: []tools.ToolDefinition{
			{
				Name:        "lookup",
				Description: "lookup data",
				Parameters: tools.ToolParameters{
					Type: "object",
					Properties: map[string]tools.ParameterProperty{
						"q": {Type: "string", Description: "query"},
					},
					Required: []string{"q"},
				},
			},
		},
		ResponseFormat: &ResponseFormat{Type: "json_object"},
	}, true)

	if req.Thinking == nil || req.Thinking.Type != "enabled" {
		t.Fatalf("thinking = %#v, want enabled", req.Thinking)
	}
	if req.ReasoningEffort != "max" {
		t.Fatalf("reasoning_effort = %q, want max", req.ReasoningEffort)
	}
	if req.StreamOptions == nil || !req.StreamOptions.IncludeUsage {
		t.Fatalf("stream_options = %#v, want include_usage", req.StreamOptions)
	}
	if len(req.Messages) != 3 {
		t.Fatalf("messages len = %d, want 3", len(req.Messages))
	}
	if req.Messages[1].ReasoningContent != "need a tool" {
		t.Fatalf("assistant reasoning_content = %q", req.Messages[1].ReasoningContent)
	}
	if len(req.Messages[1].ToolCalls) != 1 || req.Messages[1].ToolCalls[0].Function.Arguments != `{"q":"x"}` {
		t.Fatalf("assistant tool calls = %#v", req.Messages[1].ToolCalls)
	}
	if req.Messages[2].ToolCallID != "call_1" || req.Messages[2].Content != "result" {
		t.Fatalf("tool message = %#v", req.Messages[2])
	}
	if len(req.Tools) != 1 || req.Tools[0].Function.Name != "lookup" {
		t.Fatalf("tools = %#v", req.Tools)
	}
	if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_object" {
		t.Fatalf("response_format = %#v", req.ResponseFormat)
	}
}

func TestDeepSeekBuildRequestDisablesThinking(t *testing.T) {
	provider := NewDeepSeekProvider()

	req := provider.buildRequest(&ChatRequest{
		Model:         "deepseek-v4-flash",
		Thinking:      false,
		ThinkingLevel: "max",
		Messages: []Message{
			{Role: models.RoleUser, Content: "hello"},
			{Role: models.RoleAssistant, Content: "answer", ReasoningContent: "old reasoning"},
		},
	}, false)

	if req.Thinking == nil || req.Thinking.Type != "disabled" {
		t.Fatalf("thinking = %#v, want disabled", req.Thinking)
	}
	if req.ReasoningEffort != "" {
		t.Fatalf("reasoning_effort = %q, want empty", req.ReasoningEffort)
	}
	if req.StreamOptions != nil {
		t.Fatalf("stream_options = %#v, want nil", req.StreamOptions)
	}
	if req.Messages[1].ReasoningContent != "" {
		t.Fatalf("assistant reasoning_content = %q, want empty", req.Messages[1].ReasoningContent)
	}
}

func TestDeepSeekBuildRequestSanitizesInvalidUnicode(t *testing.T) {
	provider := NewDeepSeekProvider()
	bad := "system " + string([]byte{0xed, 0xa0, 0x80}) + "\x00" + string(rune(0xFDD0)) + " prompt"

	req := provider.buildRequest(&ChatRequest{
		Model:    "deepseek-v4-flash",
		Messages: []Message{{Role: models.RoleSystem, Content: bad}},
		Tools: []tools.ToolDefinition{
			{
				Name:        "bad" + string([]byte{0xff}),
				Description: "desc\x01",
				Parameters: tools.ToolParameters{
					Type: "object",
					Properties: map[string]tools.ParameterProperty{
						"arg": {Type: "string", Description: "bad" + string([]byte{0xfe})},
					},
					Required: []string{"arg"},
				},
			},
		},
	}, false)

	if !utf8.ValidString(req.Messages[0].Content) {
		t.Fatalf("content is not valid utf8: %q", req.Messages[0].Content)
	}
	if strings.ContainsRune(req.Messages[0].Content, '\x00') {
		t.Fatalf("content still contains NUL: %q", req.Messages[0].Content)
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if !utf8.Valid(body) {
		t.Fatalf("marshaled request is not valid utf8: %q", string(body))
	}
	if strings.Contains(string(body), "\\u0000") {
		t.Fatalf("marshaled request still contains NUL escape: %s", string(body))
	}
}
