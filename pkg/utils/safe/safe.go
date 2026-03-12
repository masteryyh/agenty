package safe

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/masteryyh/agenty/pkg/utils/signal"
)

type contextKey string

const goIDKey contextKey = "goID"

func GoSafe(name string, fn func(ctx context.Context)) {
	GoSafeWithCtx(name, signal.GetBaseContext(), fn)
}

func GoSafeWithCtx(name string, ctx context.Context, fn func(ctx context.Context)) {
	ctxWithGoID, cancel := createContext(ctx, name)

	go func() {
		for {
			panicked := false
			func() {
				defer func() {
					if r := recover(); r != nil {
						panicked = true
						slog.Error("recovered from panic, restarting", "goroutine", name, "error", r, "stack", string(debug.Stack()))
					}
				}()
				fn(ctxWithGoID)
			}()

			cancel()
			if !panicked {
				return
			}
			if ctx.Err() != nil {
				return
			}
			time.Sleep(500 * time.Millisecond)
			if ctx.Err() != nil {
				return
			}
			ctxWithGoID, cancel = createContext(ctx, name)
		}
	}()
}

func GoOnce(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered from panic in goroutine", "goroutine", name, "error", r, "stack", string(debug.Stack()))
			}
		}()
		fn()
	}()
}

func createContext(baseCtx context.Context, goID string) (context.Context, context.CancelFunc) {
	if baseCtx == nil {
		baseCtx = signal.GetBaseContext()
	}
	ctxWithGoID, cancel := context.WithCancel(context.WithValue(baseCtx, goIDKey, goID))
	return ctxWithGoID, cancel
}
