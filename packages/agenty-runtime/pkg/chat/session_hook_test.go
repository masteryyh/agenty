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
