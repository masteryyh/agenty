//go:build integration

package httpapi

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/masteryyh/agenty-inspector/internal/inspection"
	"github.com/masteryyh/agenty-inspector/internal/testfixture"
)

func TestChangesStreamAndShutdown(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sessions"), 0700); err != nil {
		t.Fatal(err)
	}
	store := inspection.NewStore(dir)
	store.Scan(t.Context())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	handler := New(Options{Store: store, Address: listener.Addr().String(), Assets: fstest.MapFS{}})
	server := &http.Server{Handler: handler, BaseContext: func(net.Listener) context.Context { return ctx }}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		cancel()
		shutdown, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		if err := server.Shutdown(shutdown); err != nil {
			t.Error(err)
		}
		<-done
	})
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + "/api/v1/changes")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	readGeneration := func() string {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(line, "data: ") {
				return strings.TrimSpace(strings.TrimPrefix(line, "data: "))
			}
		}
	}
	first := readGeneration()
	path := filepath.Join(dir, "sessions", "test.jsonl")
	if err := os.WriteFile(path, testfixture.Encode(testfixture.Events()), 0600); err != nil {
		t.Fatal(err)
	}
	store.Scan(t.Context())
	second := readGeneration()
	if first == second || second != fmt.Sprint(store.System().Generation) {
		t.Fatalf("generation did not advance: %s -> %s", first, second)
	}
}
