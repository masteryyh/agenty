package chat

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/providers"
	"github.com/masteryyh/agenty/pkg/utils/safe"
	"gorm.io/gorm"
)

type SessionHookPoint string

const (
	SessionHookAfterUserInput      SessionHookPoint = "afterUserInput"
	SessionHookAfterSessionCreated SessionHookPoint = "afterSessionCreated"
	SessionHookBeforeModelCall     SessionHookPoint = "beforeModelCall"
	SessionHookAfterModelResponse  SessionHookPoint = "afterModelResponse"
	SessionHookBeforeToolExecution SessionHookPoint = "beforeToolExecution"
	SessionHookAfterToolExecution  SessionHookPoint = "afterToolExecution"
	SessionHookAfterMessagesSaved  SessionHookPoint = "afterMessagesSaved"
	SessionHookAfterRoundCompleted SessionHookPoint = "afterRoundCompleted"
)

type SessionHookOptions struct {
	Async       bool
	IgnoreError bool
}

type SessionHookContext struct {
	SessionID      uuid.UUID
	AgentID        uuid.UUID
	ModelID        uuid.UUID
	RoundID        uuid.UUID
	ModelCode      string
	Cwd            string
	Iteration      int
	TotalTokens    int64
	ContextTokens  int64
	ThinkingLevel  string
	Session        *models.ChatSession
	Input          *models.ChatDto
	Params         *ChatParams
	Tx             *gorm.DB
	Request        *providers.ChatRequest
	Message        *providers.Message
	Messages       []models.ChatMessage
	ToolCall       *models.ToolCall
	ToolResult     *models.ToolResult
	SessionUpdates map[string]any
}

type SessionHookFunc func(context.Context, *SessionHookContext) error

type sessionHookEntry struct {
	point   SessionHookPoint
	name    string
	options SessionHookOptions
	fn      SessionHookFunc
}

var globalSessionHooks = &sessionHookRegistry{
	hooks: make(map[SessionHookPoint][]sessionHookEntry),
}

type sessionHookRegistry struct {
	mu    sync.RWMutex
	hooks map[SessionHookPoint][]sessionHookEntry
}

func RegisterSessionHook(point SessionHookPoint, name string, options SessionHookOptions, fn SessionHookFunc) {
	if name == "" {
		panic("session hook name is required")
	}
	if fn == nil {
		panic("session hook function is required")
	}

	globalSessionHooks.mu.Lock()
	defer globalSessionHooks.mu.Unlock()

	globalSessionHooks.hooks[point] = append(globalSessionHooks.hooks[point], sessionHookEntry{
		point:   point,
		name:    name,
		options: options,
		fn:      fn,
	})
}

func RunSessionHooks(ctx context.Context, point SessionHookPoint, hookCtx *SessionHookContext) error {
	entries := sessionHookEntries(point)
	if len(entries) == 0 {
		return nil
	}

	errs := make([]error, len(entries))
	var wg sync.WaitGroup

	for i, entry := range entries {
		idx, e := i, entry
		if e.options.Async {
			wg.Add(1)
			safe.GoOnce(fmt.Sprintf("session-hook-%s-%s", point, e.name), func() {
				defer wg.Done()
				errs[idx] = runSessionHook(ctx, e, hookCtx)
			})
			continue
		}
		errs[idx] = runSessionHook(ctx, e, hookCtx)
	}

	wg.Wait()

	for i, entry := range entries {
		if errs[i] == nil {
			continue
		}
		if entry.options.IgnoreError {
			slog.WarnContext(ctx, "ignored session hook error", "point", entry.point, "name", entry.name, "error", errs[i])
			continue
		}
		return fmt.Errorf("session hook %s/%s failed: %w", entry.point, entry.name, errs[i])
	}

	return nil
}

func sessionHookEntries(point SessionHookPoint) []sessionHookEntry {
	globalSessionHooks.mu.RLock()
	defer globalSessionHooks.mu.RUnlock()

	entries := globalSessionHooks.hooks[point]
	result := make([]sessionHookEntry, len(entries))
	copy(result, entries)
	return result
}

func runSessionHook(ctx context.Context, entry sessionHookEntry, hookCtx *SessionHookContext) (err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "panic in session hook", "point", entry.point, "name", entry.name, "error", r, "stack", string(debug.Stack()))
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return entry.fn(ctx, hookCtx)
}
