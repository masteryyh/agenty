package chat

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRunSessionHooksPreservesRegistrationOrder(t *testing.T) {
	resetSessionHooksForTest()
	defer resetSessionHooksForTest()

	var calls []string
	for _, name := range []string{"first", "second", "third"} {
		hookName := name
		RegisterSessionHook(SessionHookBeforeModelCall, hookName, SessionHookOptions{}, func(context.Context, *SessionHookContext) error {
			calls = append(calls, hookName)
			return nil
		})
	}

	if err := RunSessionHooks(context.Background(), SessionHookBeforeModelCall, &SessionHookContext{}); err != nil {
		t.Fatalf("RunSessionHooks() error = %v", err)
	}

	if !slices.Equal(calls, []string{"first", "second", "third"}) {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestRunSessionHooksWaitsForAsyncHooks(t *testing.T) {
	resetSessionHooksForTest()
	defer resetSessionHooksForTest()

	done := make(chan struct{})
	RegisterSessionHook(SessionHookBeforeModelCall, "async", SessionHookOptions{Async: true}, func(context.Context, *SessionHookContext) error {
		time.Sleep(10 * time.Millisecond)
		close(done)
		return nil
	})

	if err := RunSessionHooks(context.Background(), SessionHookBeforeModelCall, &SessionHookContext{}); err != nil {
		t.Fatalf("RunSessionHooks() error = %v", err)
	}

	select {
	case <-done:
	default:
		t.Fatal("async hook was not completed before RunSessionHooks returned")
	}
}

func TestRunSessionHooksIgnoresConfiguredErrorsAndReturnsFirstBlockingError(t *testing.T) {
	resetSessionHooksForTest()
	defer resetSessionHooksForTest()

	var calls []string
	RegisterSessionHook(SessionHookBeforeModelCall, "ignored", SessionHookOptions{IgnoreError: true}, func(context.Context, *SessionHookContext) error {
		calls = append(calls, "ignored")
		return fmt.Errorf("ignored failure")
	})
	RegisterSessionHook(SessionHookBeforeModelCall, "stop", SessionHookOptions{}, func(context.Context, *SessionHookContext) error {
		calls = append(calls, "stop")
		return fmt.Errorf("stop failure")
	})
	RegisterSessionHook(SessionHookBeforeModelCall, "after", SessionHookOptions{}, func(context.Context, *SessionHookContext) error {
		calls = append(calls, "after")
		return nil
	})

	err := RunSessionHooks(context.Background(), SessionHookBeforeModelCall, &SessionHookContext{})
	if err == nil || !strings.Contains(err.Error(), "stop failure") {
		t.Fatalf("RunSessionHooks() error = %v", err)
	}
	if !slices.Equal(calls, []string{"ignored", "stop", "after"}) {
		t.Fatalf("calls = %#v", calls)
	}
}

func resetSessionHooksForTest() {
	globalSessionHooks.mu.Lock()
	defer globalSessionHooks.mu.Unlock()

	globalSessionHooks.hooks = make(map[SessionHookPoint][]sessionHookEntry)
}
