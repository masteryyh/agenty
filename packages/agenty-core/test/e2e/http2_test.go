//go:build e2e

package e2e_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestCoreHTTP2UsesRESTEndpointsAndStableErrorCodes(t *testing.T) {
	t.Parallel()
	ctx, cancel := testContext(t)
	defer cancel()
	process := startCore(t)

	var status map[string]any
	requireNoError(t, process.Request(ctx, http.MethodGet, "/v1/system", nil, &status))
	if status["apiContract"] != "v1" || status["ipcVersion"] != "1" {
		t.Fatalf("system response = %+v", status)
	}

	var providers []Provider
	requireNoError(t, process.Request(ctx, http.MethodGet, "/v1/providers", nil, &providers))
	if len(providers) == 0 {
		t.Fatal("built-in provider list is empty")
	}

	err := process.Request(ctx, http.MethodGet, "/v1/providers/missing-provider", nil, nil)
	requireAPIError(t, err, "not_found")
	if !strings.Contains(err.Error(), "provider") {
		t.Fatalf("missing provider error = %v", err)
	}

	// A JSON-RPC envelope is not dispatched through a catch-all endpoint.
	err = process.Request(ctx, http.MethodPost, "/rpc", map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "provider.list",
	}, nil)
	if err == nil {
		t.Fatal("legacy method tunnel unexpectedly exists")
	}

	// Oversized JSON bodies fail with a stable HTTP business code; callers
	// send normal request bodies and never assemble chunk.begin/part/commit.
	large := strings.Repeat("x", (4<<20)+1)
	err = process.Request(ctx, http.MethodPost, "/v1/providers", map[string]any{
		"code":     "oversized",
		"name":     "Oversized",
		"type":     "openai",
		"metadata": map[string]any{"large": large},
	}, nil)
	requireAPIError(t, err, "invalid_params")
}

func TestCoreStartupWithClosedStdinStillServesUntilHTTPShutdown(t *testing.T) {
	t.Parallel()
	ctx, cancel := testContext(t)
	defer cancel()
	process := startCore(t)

	var info map[string]any
	requireNoError(t, process.Request(ctx, http.MethodGet, "/v1/system", nil, &info))
	if info["processId"] == nil {
		t.Fatalf("system response omitted processId: %+v", info)
	}
	requireNoError(t, process.Close())
}

func TestCoreHTTP2RejectsMalformedJSONBodyWithoutLosingConnection(t *testing.T) {
	t.Parallel()
	ctx, cancel := testContext(t)
	defer cancel()
	process := startCore(t)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://agenty.local/v1/providers", strings.NewReader("{"))
	requireNoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	response, err := process.client.Do(request)
	requireNoError(t, err)
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed body status = %d, want 400", response.StatusCode)
	}

	var providers []Provider
	requireNoError(t, process.Request(ctx, http.MethodGet, "/v1/providers", nil, &providers))
	if len(providers) == 0 {
		t.Fatal("connection did not recover after malformed request")
	}
}
