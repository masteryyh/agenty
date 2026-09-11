package mcp

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	domainmcp "github.com/masteryyh/agenty-core/pkg/domain/mcp"
)

func TestRegistryServerLogsAreBoundedAndSanitized(t *testing.T) {
	registry, err := NewRegistry(
		context.Background(),
		t.TempDir(),
		agentloop.NewRegistry(),
		Options{},
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	defer func() {
		if err := registry.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	}()

	config := domainmcp.Config{
		Type:    domainmcp.TransportStdio,
		Enabled: false,
		Command: "node",
		Env:     map[string]string{"TOKEN": "secret-token"},
	}
	if _, err := registry.Create(t.Context(), "stdio", config); err != nil {
		t.Fatalf("Create: %v", err)
	}

	writer := &stdioLogWriter{registry: registry, name: "stdio", generation: 0}
	if _, err := writer.Write([]byte("secret-token\t\x1b[31mfailed\x1b[0m\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	logs, ok := registry.Logs(t.Context(), "stdio")
	if !ok || len(logs) != 1 {
		t.Fatalf("initial logs = %#v, ok=%v", logs, ok)
	}
	if strings.Contains(logs[0].Message, "secret-token") || strings.ContainsAny(logs[0].Message, "\x1b\t") {
		t.Fatalf("log message was not sanitized: %q", logs[0].Message)
	}

	for index := 0; index < maxServerLogEntries+5; index++ {
		registry.appendServerLog("stdio", 0, "info", "server", "stderr", fmt.Sprintf("entry-%d", index))
	}
	logs, ok = registry.Logs(t.Context(), "stdio")
	if !ok || len(logs) != maxServerLogEntries {
		t.Fatalf("bounded logs = %d, ok=%v, want %d", len(logs), ok, maxServerLogEntries)
	}
	if logs[0].Message != "entry-5" || logs[len(logs)-1].Message != "entry-54" {
		t.Fatalf("bounded log window = %q ... %q", logs[0].Message, logs[len(logs)-1].Message)
	}
}
