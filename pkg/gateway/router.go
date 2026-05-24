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

package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/customerrors"
	gatewaychannel "github.com/masteryyh/agenty/pkg/gateway/channel"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/providers"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/utils/safe"
)

type Router struct {
	gatewayService *services.GatewayService
	chatService    *services.ChatService
	renderer       *EventRenderer
	locks          map[string]*conversationLock
	locksMu        sync.Mutex
}

type conversationLock struct {
	mu       sync.Mutex
	refCount int
}

func NewRouter() *Router {
	return &Router{
		gatewayService: services.GetGatewayService(),
		chatService:    services.GetChatService(),
		renderer:       NewEventRenderer(),
		locks:          make(map[string]*conversationLock),
	}
}

func (r *Router) HandleInbound(ctx context.Context, adapter gatewaychannel.Adapter, msg gatewaychannel.InboundMessage) error {
	msg.Text = strings.TrimSpace(msg.Text)
	if msg.Text == "" {
		return nil
	}

	channelCfg, err := r.gatewayService.GetChannel(ctx, msg.ChannelID)
	if err != nil {
		if errors.Is(err, customerrors.ErrGatewayChannelNotFound) {
			return nil
		}
		return err
	}
	if !channelCfg.Enabled {
		return nil
	}
	if channelCfg.RequireMention && !msg.MentionsBot {
		return nil
	}

	if msg.ID != "" {
		delivery, err := r.gatewayService.FindMessageDelivery(ctx, msg.ChannelID, msg.AccountID, msg.ID)
		if err != nil {
			return err
		}
		if delivery != nil {
			return nil
		}
	}

	binding, err := r.gatewayService.ResolveBinding(ctx, msg.ChannelID, msg.AccountID)
	if err != nil {
		if errors.Is(err, customerrors.ErrGatewayBindingNotFound) {
			return nil
		}
		return err
	}

	lockKey := fmt.Sprintf("%s:%s:%s:%s:%s", msg.ChannelID, msg.AccountID, msg.ConversationID, msg.SenderID, binding.AgentID.String())
	lock := r.acquireConversationLock(lockKey)
	lock.mu.Lock()
	defer func() {
		lock.mu.Unlock()
		r.releaseConversationLock(lockKey)
	}()

	conversation, err := r.gatewayService.GetConversation(ctx, msg.ChannelID, msg.AccountID, msg.ConversationID, msg.SenderID, binding.AgentID)
	if err != nil {
		return err
	}

	var sessionID uuid.UUID
	var modelID uuid.UUID
	if conversation == nil {
		session, err := r.chatService.CreateSession(ctx, &models.CreateSessionDto{AgentID: binding.AgentID})
		if err != nil {
			return err
		}
		sessionID = session.ID
		modelID = session.LastUsedModel
		conversation = &models.GatewayConversation{
			BindingID:      &binding.ID,
			ChannelID:      msg.ChannelID,
			ChannelType:    msg.ChannelType,
			AccountID:      msg.AccountID,
			ConversationID: msg.ConversationID,
			SenderID:       msg.SenderID,
			AgentID:        binding.AgentID,
			SessionID:      sessionID,
		}
		if err := r.gatewayService.CreateConversation(ctx, conversation); err != nil {
			return err
		}
	} else {
		sessionID = conversation.SessionID
		modelID, err = r.gatewayService.GetSessionModelID(ctx, sessionID)
		if err != nil {
			return err
		}
	}

	if binding.DefaultModelID != nil {
		modelID = *binding.DefaultModelID
	}

	var delivery *models.GatewayMessageDelivery
	if msg.ID != "" {
		delivery = &models.GatewayMessageDelivery{
			BindingID:         &binding.ID,
			ChannelID:         msg.ChannelID,
			AccountID:         msg.AccountID,
			ExternalMessageID: msg.ID,
			ConversationID:    msg.ConversationID,
			AgentID:           binding.AgentID,
			SessionID:         sessionID,
			Status:            "processing",
		}
		if err := r.gatewayService.CreateMessageDelivery(ctx, delivery); err != nil {
			return err
		}
	}

	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()
	stream, err := r.chatService.StreamChat(streamCtx, sessionID, &models.ChatDto{
		ModelID: modelID,
		Message: msg.Text,
	})
	if err != nil {
		r.finishDelivery(ctx, delivery, "failed", err.Error())
		_ = adapter.Send(ctx, gatewaychannel.OutboundEvent{
			Type:           gatewaychannel.OutboundError,
			ChannelID:      msg.ChannelID,
			ChannelType:    msg.ChannelType,
			AccountID:      msg.AccountID,
			ConversationID: msg.ConversationID,
			Text:           fmt.Sprintf("Error: %s", err.Error()),
			Final:          true,
		})
		return err
	}

	finalStatus := "done"
	finalError := ""
	for evt := range stream {
		if evt.Type == providers.EventError {
			finalStatus = "failed"
			finalError = evt.Error
		}
		for _, outbound := range r.renderer.Render(channelCfg, msg, evt) {
			if sendErr := adapter.Send(ctx, outbound); sendErr != nil {
				finalStatus = "failed"
				finalError = sendErr.Error()
				r.finishDelivery(ctx, delivery, finalStatus, finalError)
				streamCancel()
				safe.GoOnce("gateway-stream-drain", func() {
					for range stream {
					}
				})
				return sendErr
			}
		}
	}

	if err := r.gatewayService.TouchConversation(ctx, conversation.ID); err != nil {
		slog.WarnContext(ctx, "failed to touch gateway conversation", "error", err, "conversationId", conversation.ID)
	}
	r.finishDelivery(ctx, delivery, finalStatus, finalError)
	return nil
}

func (r *Router) acquireConversationLock(key string) *conversationLock {
	r.locksMu.Lock()
	defer r.locksMu.Unlock()
	lock, ok := r.locks[key]
	if !ok {
		lock = &conversationLock{}
		r.locks[key] = lock
	}
	lock.refCount++
	return lock
}

func (r *Router) releaseConversationLock(key string) {
	r.locksMu.Lock()
	defer r.locksMu.Unlock()
	lock, ok := r.locks[key]
	if !ok {
		return
	}
	lock.refCount--
	if lock.refCount <= 0 {
		delete(r.locks, key)
	}
}

func (r *Router) finishDelivery(ctx context.Context, delivery *models.GatewayMessageDelivery, status, errorText string) {
	if delivery == nil {
		return
	}
	finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.gatewayService.FinishMessageDelivery(finishCtx, delivery.ID, status, errorText); err != nil {
		slog.WarnContext(ctx, "failed to finish gateway message delivery", "error", err, "deliveryId", delivery.ID)
	}
}
