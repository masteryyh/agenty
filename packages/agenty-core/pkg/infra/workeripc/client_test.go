package workeripc

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWorkerProcessFixture(t *testing.T) {
	if os.Getenv("AGENTY_WORKER_FIXTURE") != "1" {
		return
	}
	input := bufio.NewScanner(os.Stdin)
	var generation string
	send := func(id string, fields map[string]any) {
		fields["version"] = 1
		fields["requestId"] = id
		if fields["type"] == nil {
			fields["type"] = "response"
		}
		_ = json.NewEncoder(os.Stdout).Encode(fields)
	}
	for input.Scan() {
		var frame struct {
			RequestID, Method string
			Params            json.RawMessage
		}
		_ = json.Unmarshal(input.Bytes(), &frame)
		switch frame.Method {
		case "initialize":
			send(frame.RequestID, map[string]any{"result": map[string]any{"protocolVersion": 1}})
		case "generate":
			generation = frame.RequestID
			send(generation, map[string]any{"type": "event", "event": "tokens", "sequence": 1, "data": map[string]any{"tokenIds": []int{1}}})
		case "cancel":
			send(frame.RequestID, map[string]any{"result": map[string]any{"accepted": true}})
			send(generation, map[string]any{"sequence": 2, "result": map[string]any{"finishReason": "cancelled"}})
		case "bad":
			_, _ = os.Stdout.WriteString("{invalid}\n")
		case "completeThenExit":
			send(frame.RequestID, map[string]any{"result": map[string]any{"ok": true}})
			os.Exit(0)
		case "unterminatedResponse":
			data, _ := json.Marshal(map[string]any{"version": 1, "type": "response", "requestId": frame.RequestID, "result": map[string]any{"ok": true}})
			_, _ = os.Stdout.Write(data)
			os.Exit(0)
		case "shutdown":
			send(frame.RequestID, map[string]any{"result": map[string]any{}})
			os.Exit(0)
		}
	}
	os.Exit(0)
}

func fixtureClient(t *testing.T) *Client {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	client, err := Start(t.Context(), executable, []string{"-test.run=^TestWorkerProcessFixture$"}, append(os.Environ(), "AGENTY_WORKER_FIXTURE=1"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = client.Close(ctx)
	})
	return client
}

func TestCancellationDrainsTerminalAndKeepsWorkerReusable(t *testing.T) {
	client := fixtureClient(t)
	ctx, cancel := context.WithCancel(t.Context())
	err := client.Call(ctx, "generate", struct{}{}, nil, func(frame Frame) error { cancel(); return nil })
	if err != context.Canceled {
		t.Fatalf("cancel result: %v", err)
	}
	var response struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	if err := client.Call(t.Context(), "initialize", struct{}{}, &response, nil); err != nil || response.ProtocolVersion != 1 {
		t.Fatal(response, err)
	}
}
func TestMalformedFrameFailsPendingCall(t *testing.T) {
	client := fixtureClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := client.Call(ctx, "bad", struct{}{}, nil, nil); err == nil || !strings.Contains(err.Error(), "invalid frame") {
		t.Fatalf("invalid frame result: %v", err)
	}
}

func TestTerminalIsDeliveredBeforeWorkerEOF(t *testing.T) {
	client := fixtureClient(t)
	var result struct {
		OK bool `json:"ok"`
	}
	if err := client.Call(t.Context(), "completeThenExit", struct{}{}, &result, nil); err != nil || !result.OK {
		t.Fatal(result, err)
	}
}

func TestWorkerRejectsUnterminatedJSONFrame(t *testing.T) {
	client := fixtureClient(t)
	if err := client.Call(t.Context(), "unterminatedResponse", struct{}{}, nil, nil); err == nil {
		t.Fatal("accepted a frame without the required LF terminator")
	}
}
