//go:build windows

package instance

import (
	"context"
	"fmt"
	"net"

	winio "github.com/Microsoft/go-winio"
)

func Listen(transport, address string) (net.Listener, error) {
	if transport != "named_pipe" {
		return nil, fmt.Errorf("transport %q is not supported on Windows", transport)
	}
	return winio.ListenPipe(address, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;OW)",
		MessageMode:        false,
	})
}

func Dial(ctx context.Context, transport, address string) (net.Conn, error) {
	if transport != "named_pipe" {
		return nil, fmt.Errorf("transport %q is not supported on Windows", transport)
	}
	return winio.DialPipeContext(ctx, address)
}
