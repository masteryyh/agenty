//go:build windows

package instance

import "context"

// Windows child ownership is enforced by the bootstrap Job Object.
func MonitorParent(context.Context, context.CancelFunc) {}
