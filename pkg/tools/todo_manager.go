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

package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/safe"
)

type sessionTodoStore struct {
	mu        sync.Mutex
	items     []*models.TodoItemDto
	nextID    int
	createdAt time.Time
}

const (
	sessionStoreTTL = 24 * time.Hour
	cleanupInterval = 30 * time.Minute
)

type TodoManager struct {
	mu     sync.RWMutex
	stores map[uuid.UUID]*sessionTodoStore
}

var (
	todoManager     *TodoManager
	todoManagerOnce sync.Once
)

func GetTodoManager() *TodoManager {
	todoManagerOnce.Do(func() {
		todoManager = &TodoManager{
			stores: make(map[uuid.UUID]*sessionTodoStore),
		}
		safe.GoSafe("todo-list-cleanup", func(ctx context.Context) {
			todoManager.runCleanup(ctx)
		})
	})
	return todoManager
}

func (m *TodoManager) runCleanup(ctx context.Context) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.evictExpired()
		}
	}
}

func (m *TodoManager) evictExpired() {
	cutoff := time.Now().Add(-sessionStoreTTL)
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, store := range m.stores {
		store.mu.Lock()
		expired := store.createdAt.Before(cutoff)
		store.mu.Unlock()
		if expired {
			delete(m.stores, id)
		}
	}
}

func (m *TodoManager) getStore(sessionID uuid.UUID, create bool) *sessionTodoStore {
	m.mu.RLock()
	store, ok := m.stores[sessionID]
	m.mu.RUnlock()
	if ok {
		return store
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if store, ok = m.stores[sessionID]; ok {
		return store
	}

	if !create {
		return nil
	}
	store = &sessionTodoStore{nextID: 1, createdAt: time.Now()}
	m.stores[sessionID] = store
	return store
}

func (m *TodoManager) Add(sessionID uuid.UUID, items []string) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("items cannot be empty for action 'add'")
	}

	store := m.getStore(sessionID, true)
	store.mu.Lock()
	defer store.mu.Unlock()

	var sb strings.Builder
	added := 0
	for _, content := range items {
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		item := &models.TodoItemDto{
			ID:      store.nextID,
			Content: content,
			Status:  "pending",
		}
		store.items = append(store.items, item)
		store.nextID++
		fmt.Fprintf(&sb, "  [%d] %s\n", item.ID, item.Content)
		added++
	}
	if added == 0 {
		return "", fmt.Errorf("all items were empty")
	}
	return fmt.Sprintf("Added %d todo item(s):\n%s", added, sb.String()), nil
}

func (m *TodoManager) Update(sessionID uuid.UUID, id int, status string) (string, error) {
	if status != "pending" && status != "in_progress" && status != "done" {
		return "", fmt.Errorf("invalid status %q: must be 'pending', 'in_progress', or 'done'", status)
	}

	store := m.getStore(sessionID, false)
	if store == nil {
		return "", fmt.Errorf("todo item with ID %d not found", id)
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	for _, item := range store.items {
		if item.ID == id {
			item.Status = status
			return fmt.Sprintf("Todo [%d] %q → %s", item.ID, item.Content, status), nil
		}
	}
	return "", fmt.Errorf("todo item with ID %d not found", id)
}

func (m *TodoManager) List(sessionID uuid.UUID) []models.TodoItemDto {
	store := m.getStore(sessionID, false)
	if store == nil {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.items) == 0 {
		return nil
	}
	result := make([]models.TodoItemDto, len(store.items))
	for i, item := range store.items {
		result[i] = *item
	}
	return result
}

func (m *TodoManager) FormatList(sessionID uuid.UUID) string {
	store := m.getStore(sessionID, false)
	if store == nil {
		return "Todo list is empty."
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.items) == 0 {
		return "Todo list is empty."
	}

	pending, inProgress, done := 0, 0, 0
	for _, item := range store.items {
		switch item.Status {
		case "pending":
			pending++
		case "in_progress":
			inProgress++
		case "done":
			done++
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Todo list (%d total — %d pending, %d in_progress, %d done):\n\n", len(store.items), pending, inProgress, done)
	for _, item := range store.items {
		var marker string
		switch item.Status {
		case "pending":
			marker = "[ ]"
		case "in_progress":
			marker = "[~]"
		case "done":
			marker = "[x]"
		}
		fmt.Fprintf(&sb, "%s [%d] %s\n", marker, item.ID, item.Content)
	}
	return sb.String()
}
