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

package provider

import "github.com/masteryyh/agenty/pkg/models"

type StreamEventType string

const (
	EventReasoningDelta StreamEventType = "reasoning_delta"
	EventContentDelta   StreamEventType = "content_delta"
	EventToolCallStart  StreamEventType = "tool_call_start"
	EventToolCallDelta  StreamEventType = "tool_call_delta"
	EventToolCallDone   StreamEventType = "tool_call_done"
	EventToolResult     StreamEventType = "tool_result"
	EventMessageDone    StreamEventType = "message_done"
	EventUsage          StreamEventType = "usage"
	EventError          StreamEventType = "error"
	EventDone           StreamEventType = "done"
)

type StreamEvent struct {
	Type       StreamEventType    `json:"type"`
	Content    string             `json:"content,omitempty"`
	Reasoning  string             `json:"reasoning,omitempty"`
	ToolCall   *models.ToolCall   `json:"toolCall,omitempty"`
	ToolResult *models.ToolResult `json:"toolResult,omitempty"`
	Usage      *StreamUsage       `json:"usage,omitempty"`
	Error      string             `json:"error,omitempty"`
	Message    *Message           `json:"message,omitempty"`
}

type StreamUsage struct {
	TotalTokens int64 `json:"totalTokens"`
}
