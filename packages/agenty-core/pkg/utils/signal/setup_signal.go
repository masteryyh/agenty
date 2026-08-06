package signal

import (
	"context"
	"os"
	"os/signal"
	"sync"
)

var (
	baseCtx    context.Context
	baseCancel context.CancelFunc
	once       sync.Once
)

func SetupContext() (context.Context, context.CancelFunc) {
	once.Do(func() {
		baseCtx, baseCancel = context.WithCancel(context.Background())
		c := make(chan os.Signal, 2)
		signal.Notify(c, shutdownSignals...)

		go func() {
			<-c
			baseCancel()
			<-c
			os.Exit(1)
		}()
	})
	return baseCtx, baseCancel
}

func GetBaseContext() context.Context {
	if baseCtx == nil {
		panic("base context is not initialized, call SetupContext() first")
	}
	return baseCtx
}
