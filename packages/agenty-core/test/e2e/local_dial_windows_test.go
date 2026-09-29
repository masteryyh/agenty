//go:build e2e && windows

package e2e_test

import (
	"context"
	"net"

	winio "github.com/Microsoft/go-winio"
)

func dialLocal(ctx context.Context, transport, address string) (net.Conn, error) {
	if transport != "named_pipe" {
		return nil, net.UnknownNetworkError(transport)
	}
	return winio.DialPipeContext(ctx, address)
}
