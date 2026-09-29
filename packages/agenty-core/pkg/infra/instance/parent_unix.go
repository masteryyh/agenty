//go:build !windows

package instance

import (
	"context"
	"os"
	"time"
)

// MonitorParent cancels the core if the bootstrap process disappears. Unix
// reassigns an orphaned process to init/launchd, so the original parent id is
// enough to detect both an abrupt exit and a crash without an extra env var.
func MonitorParent(ctx context.Context, cancel context.CancelFunc) {
	parentPID := os.Getppid()
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if os.Getppid() != parentPID {
					cancel()
					return
				}
			}
		}
	}()
}
