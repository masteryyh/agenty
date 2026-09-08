package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/masteryyh/agenty-core/pkg/infra/config"
	inspector "github.com/masteryyh/agenty-inspector"
	"github.com/masteryyh/agenty-inspector/internal/httpapi"
	"github.com/masteryyh/agenty-inspector/internal/inspection"
)

func main() {
	if err := run(); err != nil {
		slog.Error("inspector stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	port := flag.Int("port", 4318, "local HTTP port (0 selects an available port)")
	open := flag.Bool("open", false, "open the web application in your browser")
	devOrigin := flag.String("dev-origin", "", "allowed local frontend development origin")
	flag.Parse()
	if *port < 0 || *port > 65535 {
		return fmt.Errorf("invalid port %d", *port)
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	dataDir, err := filepath.Abs(paths.DataDir)
	if err != nil {
		return fmt.Errorf("resolve data directory: %w", err)
	}
	assets, err := inspector.Assets()
	if err != nil {
		return fmt.Errorf("load web assets: %w", err)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store := inspection.NewStore(dataDir)
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		store.Run(ctx)
	}()

	server := &http.Server{
		Handler: httpapi.New(httpapi.Options{
			Store:     store,
			Assets:    assets,
			Address:   listener.Addr().String(),
			DevOrigin: *devOrigin,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	address := "http://" + listener.Addr().String()
	fmt.Printf("Agenty Inspector\n  Web:  %s\n  Data: %s\n  Mode: read-only\n", address, dataDir)

	done := make(chan error, 1)
	go func() {
		done <- server.Serve(listener)
	}()

	if *open {
		if err := openBrowser(ctx, address); err != nil {
			slog.WarnContext(ctx, "could not open browser", "error", err)
		}
	}

	select {
	case err := <-done:
		cancel()
		<-scanDone
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := server.Shutdown(shutdown)

		<-scanDone
		return err
	}
}

func openBrowser(ctx context.Context, address string) error {
	command, args := "xdg-open", []string{address}
	switch runtime.GOOS {
	case "darwin":
		command = "open"
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", address}
	}
	return exec.CommandContext(ctx, command, args...).Run()
}
