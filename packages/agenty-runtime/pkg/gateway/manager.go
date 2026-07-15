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
	"fmt"
	"log/slog"
	"sync"

	discordadapter "github.com/masteryyh/agenty/pkg/gateway/adapters/discord"
	gatewaychannel "github.com/masteryyh/agenty/pkg/gateway/channel"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/services"
	"github.com/masteryyh/agenty/pkg/utils/signal"
)

type Manager struct {
	router   *Router
	service  *services.GatewayService
	adapters []gatewaychannel.Adapter
	stopped  bool
	mu       sync.Mutex
}

var (
	activeManager *Manager
	activeMu      sync.RWMutex
)

func NewManager() *Manager {
	return &Manager{
		router:  NewRouter(),
		service: services.GetGatewayService(),
	}
}

func ReloadActiveManager(ctx context.Context) error {
	activeMu.RLock()
	manager := activeManager
	activeMu.RUnlock()
	if manager == nil {
		return nil
	}
	logCtx := ctx
	if logCtx == nil {
		logCtx = context.Background()
	}
	reloadCtx := context.WithoutCancel(logCtx)
	if err := manager.Reload(reloadCtx); err != nil {
		slog.WarnContext(logCtx, "failed to reload gateway manager", "error", err)
		return err
	}
	return nil
}

func ReloadActiveManagerWithChannels(ctx context.Context, channels []models.GatewayChannel) error {
	activeMu.RLock()
	manager := activeManager
	activeMu.RUnlock()
	if manager == nil {
		return nil
	}
	logCtx := ctx
	if logCtx == nil {
		logCtx = signal.GetBaseContext()
	}
	reloadCtx := context.WithoutCancel(logCtx)
	if err := manager.ReloadChannels(reloadCtx, channels); err != nil {
		slog.WarnContext(logCtx, "failed to reload gateway manager", "error", err)
		return err
	}
	return nil
}

func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	m.stopped = false
	m.mu.Unlock()
	activeMu.Lock()
	activeManager = m
	activeMu.Unlock()
	if err := m.Reload(ctx); err != nil {
		activeMu.Lock()
		if activeManager == m {
			activeManager = nil
		}
		activeMu.Unlock()
		return err
	}
	return nil
}

func (m *Manager) Reload(ctx context.Context) error {
	channels, err := m.service.ListEnabledChannelConfigs(ctx)
	if err != nil {
		return err
	}
	return m.ReloadChannels(ctx, channels)
}

func (m *Manager) ReloadChannels(ctx context.Context, channels []models.GatewayChannel) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return nil
	}

	nextAdapters := make([]gatewaychannel.Adapter, 0, len(channels))
	for i := range channels {
		ch := channels[i]
		if string(ch.Type) == "cli" {
			slog.WarnContext(ctx, "ignoring legacy gateway channel type", "channelId", ch.ID, "type", ch.Type)
			continue
		}
		adapter, err := m.buildAdapter(&ch)
		if err != nil {
			if ch.Required {
				stopAdapters(ctx, nextAdapters)
				return err
			}
			slog.WarnContext(ctx, "failed to build gateway adapter", "channelId", ch.ID, "error", err)
			continue
		}
		if err := adapter.Start(ctx, m.router); err != nil {
			if ch.Required {
				if stopErr := adapter.Stop(ctx); stopErr != nil {
					slog.WarnContext(ctx, "failed to stop gateway adapter after start error", "channelId", adapter.ID(), "error", stopErr)
				}
				stopAdapters(ctx, nextAdapters)
				return err
			}
			if stopErr := adapter.Stop(ctx); stopErr != nil {
				slog.WarnContext(ctx, "failed to stop gateway adapter after start error", "channelId", adapter.ID(), "error", stopErr)
			}
			slog.WarnContext(ctx, "failed to start gateway adapter", "channelId", ch.ID, "error", err)
			continue
		}
		nextAdapters = append(nextAdapters, adapter)
		slog.InfoContext(ctx, "gateway adapter started", "channelId", ch.ID, "type", ch.Type, "accountId", ch.AccountID)
	}
	oldAdapters := m.adapters
	m.adapters = nextAdapters
	stopAdapters(ctx, oldAdapters)
	return nil
}

func (m *Manager) Stop(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	m.stopLocked(ctx)
	activeMu.Lock()
	if activeManager == m {
		activeManager = nil
	}
	activeMu.Unlock()
}

func (m *Manager) stopLocked(ctx context.Context) {
	stopAdapters(ctx, m.adapters)
	m.adapters = nil
}

func stopAdapters(ctx context.Context, adapters []gatewaychannel.Adapter) {
	for i := len(adapters) - 1; i >= 0; i-- {
		adapter := adapters[i]
		if err := adapter.Stop(ctx); err != nil {
			slog.WarnContext(ctx, "failed to stop gateway adapter", "channelId", adapter.ID(), "error", err)
		}
	}
}

func (m *Manager) buildAdapter(ch *models.GatewayChannel) (gatewaychannel.Adapter, error) {
	switch ch.Type {
	case models.ChannelTypeDiscord:
		return discordadapter.New(ch)
	default:
		return nil, fmt.Errorf("unsupported gateway channel type: %s", ch.Type)
	}
}
