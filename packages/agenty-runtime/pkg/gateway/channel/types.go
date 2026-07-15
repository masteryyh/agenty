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

package channel

import (
	"context"
	"time"

	"github.com/masteryyh/agenty/pkg/models"
)

type Attachment struct {
	ID          string
	FileName    string
	ContentType string
	URL         string
}

type InboundMessage struct {
	ID               string
	ChannelID        string
	ChannelType      models.ChannelType
	AccountID        string
	ConversationID   string
	SenderID         string
	SenderName       string
	Text             string
	MentionsBot      bool
	ReplyToMessageID string
	Attachments      []Attachment
	Raw              []byte
	ReceivedAt       time.Time
}

type OutboundEventType string

const (
	OutboundMessageDelta  OutboundEventType = "message_delta"
	OutboundReasoning     OutboundEventType = "reasoning_delta"
	OutboundToolCallStart OutboundEventType = "tool_call_start"
	OutboundToolCallDone  OutboundEventType = "tool_call_done"
	OutboundToolResult    OutboundEventType = "tool_result"
	OutboundMessageDone   OutboundEventType = "message_done"
	OutboundError         OutboundEventType = "error"
)

type OutboundEvent struct {
	Type             OutboundEventType
	ChannelID        string
	ChannelType      models.ChannelType
	AccountID        string
	ConversationID   string
	ReplyToMessageID string
	Text             string
	ToolCall         *models.ToolCall
	ToolResult       *models.ToolResult
	Final            bool
}

type InboundHandler interface {
	HandleInbound(ctx context.Context, adapter Adapter, msg InboundMessage) error
}

type Adapter interface {
	ID() string
	Type() models.ChannelType
	AccountID() string
	Start(ctx context.Context, handler InboundHandler) error
	Stop(ctx context.Context) error
	Send(ctx context.Context, event OutboundEvent) error
}
