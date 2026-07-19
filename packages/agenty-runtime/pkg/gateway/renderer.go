package gateway

import (
	"fmt"
	"strings"

	gatewaychannel "github.com/masteryyh/agenty/pkg/gateway/channel"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/providers"
)

type EventRenderer struct{}

func NewEventRenderer() *EventRenderer {
	return &EventRenderer{}
}

func (r *EventRenderer) Render(channel *models.GatewayChannelDto, msg gatewaychannel.InboundMessage, evt providers.StreamEvent) []gatewaychannel.OutboundEvent {
	base := gatewaychannel.OutboundEvent{
		ChannelID:        msg.ChannelID,
		ChannelType:      msg.ChannelType,
		AccountID:        msg.AccountID,
		ConversationID:   msg.ConversationID,
		ReplyToMessageID: msg.ID,
	}

	switch evt.Type {
	case providers.EventContentDelta:
		if strings.TrimSpace(evt.Content) == "" {
			return nil
		}
		base.Type = gatewaychannel.OutboundMessageDelta
		base.Text = evt.Content
		return []gatewaychannel.OutboundEvent{base}
	case providers.EventReasoningDelta:
		if channel == nil || !channel.SendReasoning || strings.TrimSpace(evt.Reasoning) == "" {
			return nil
		}
		base.Type = gatewaychannel.OutboundReasoning
		base.Text = evt.Reasoning
		return []gatewaychannel.OutboundEvent{base}
	case providers.EventToolCallStart:
		if channel == nil || !channel.SendToolEvents || evt.ToolCall == nil {
			return nil
		}
		base.Type = gatewaychannel.OutboundToolCallStart
		base.Text = fmt.Sprintf("Running tool: %s", evt.ToolCall.Name)
		base.ToolCall = evt.ToolCall
		return []gatewaychannel.OutboundEvent{base}
	case providers.EventToolCallDone:
		if channel == nil || !channel.SendToolEvents || evt.ToolCall == nil {
			return nil
		}
		base.Type = gatewaychannel.OutboundToolCallDone
		base.Text = fmt.Sprintf("Tool finished: %s", evt.ToolCall.Name)
		base.ToolCall = evt.ToolCall
		return []gatewaychannel.OutboundEvent{base}
	case providers.EventToolResult:
		if channel == nil || !channel.SendToolEvents || evt.ToolResult == nil {
			return nil
		}
		base.Type = gatewaychannel.OutboundToolResult
		base.Text = fmt.Sprintf("Tool result: %s", evt.ToolResult.Name)
		base.ToolResult = evt.ToolResult
		return []gatewaychannel.OutboundEvent{base}
	case providers.EventMessageDone:
		if evt.Message == nil || strings.TrimSpace(evt.Message.Content) == "" {
			return nil
		}
		base.Type = gatewaychannel.OutboundMessageDone
		base.Text = evt.Message.Content
		base.Final = true
		return []gatewaychannel.OutboundEvent{base}
	case providers.EventError:
		if strings.TrimSpace(evt.Error) == "" {
			return nil
		}
		base.Type = gatewaychannel.OutboundError
		base.Text = fmt.Sprintf("Error: %s", evt.Error)
		base.Final = true
		return []gatewaychannel.OutboundEvent{base}
	default:
		return nil
	}
}
