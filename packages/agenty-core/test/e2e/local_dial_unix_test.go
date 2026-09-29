//go:build e2e && !windows

package e2e_test

import (
	"context"
	"net"
)

func dialLocal(ctx context.Context, transport, address string) (net.Conn, error) {
	if transport != "uds" {
		return nil, net.UnknownNetworkError(transport)
	}
	return (&net.Dialer{}).DialContext(ctx, "unix", address)
}
