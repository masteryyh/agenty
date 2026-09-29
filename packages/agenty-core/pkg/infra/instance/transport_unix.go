//go:build !windows

package instance

import (
	"context"
	"fmt"
	"net"
	"os"
)

func Listen(transport, address string) (net.Listener, error) {
	if transport != "uds" {
		return nil, fmt.Errorf("transport %q is not supported on this platform", transport)
	}
	_ = os.Remove(address)
	listener, err := net.Listen("unix", address)
	if err != nil {
		return nil, fmt.Errorf("listen on Unix domain socket %s: %w", address, err)
	}
	if err := os.Chmod(address, 0o600); err != nil {
		listener.Close()
		_ = os.Remove(address)
		return nil, fmt.Errorf("restrict Unix socket permissions: %w", err)
	}
	return &unlinkListener{Listener: listener, path: address}, nil
}

type unlinkListener struct {
	net.Listener
	path string
}

func (listener *unlinkListener) Close() error {
	err := listener.Listener.Close()
	removeErr := os.Remove(listener.path)
	if os.IsNotExist(removeErr) {
		removeErr = nil
	}
	if err != nil {
		return err
	}
	return removeErr
}

func Dial(ctx context.Context, transport, address string) (net.Conn, error) {
	if transport != "uds" {
		return nil, fmt.Errorf("transport %q is not supported on this platform", transport)
	}
	return (&net.Dialer{}).DialContext(ctx, "unix", address)
}
