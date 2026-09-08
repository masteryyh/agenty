package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-inspector/internal/inspection"
	"github.com/masteryyh/agenty-inspector/internal/testfixture"
)

func testAPI(t *testing.T) (http.Handler, *inspection.Store, inspection.SessionEntry) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sessions"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sessions", "example.jsonl"), testfixture.Encode(testfixture.Events()), 0600); err != nil {
		t.Fatal(err)
	}
	store := inspection.NewStore(dir)
	store.Scan(t.Context())
	assets := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>Inspector</title>")}}
	handler := New(Options{Store: store, Assets: assets, Address: "127.0.0.1:4318"})
	return handler, store, store.List("", "", "", "", false)[0]
}

func request(handler http.Handler, method, target, host, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func TestReadAPIContractAndPagination(t *testing.T) {
	t.Parallel()
	handler, _, entry := testAPI(t)
	response := request(handler, "GET", "/api/v1/sessions/"+entry.ID, "127.0.0.1:4318", "")
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var detail inspection.Detail
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/sessions/" + entry.ID
	tests := []struct {
		name   string
		target string
		status int
	}{
		{name: "static assets", target: "/", status: 200},
		{name: "system", target: "/api/v1/system", status: 200},
		{name: "list", target: "/api/v1/sessions?limit=1", status: 200},
		{name: "events", target: base + "/events?revision=" + detail.Revision + "&limit=2", status: 200},
		{name: "record", target: base + "/records/0?revision=" + detail.Revision, status: 200},
		{name: "context", target: base + "/context?revision=" + detail.Revision + "&atRecord=0", status: 200},
		{name: "diagnostics", target: base + "/diagnostics?revision=" + detail.Revision, status: 200},
		{name: "messages", target: base + "/rounds/" + detail.Rounds[0].ID + "/messages?revision=" + detail.Revision, status: 200},
		{name: "missing revision", target: base + "/events", status: 400},
		{name: "invalid limit", target: base + "/events?limit=0", status: 400},
		{name: "negative offset", target: base + "/events?offset=-1", status: 400},
		{name: "excess limit", target: base + "/events?limit=999", status: 400},
		{name: "expired snapshot", target: base + "?revision=missing", status: 409},
		{name: "unknown session", target: "/api/v1/sessions/missing", status: 404},
		{name: "unknown record", target: base + "/records/missing?revision=" + detail.Revision, status: 404},
		{name: "unknown API", target: "/api/v1/unknown", status: 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := request(handler, "GET", tt.target, "127.0.0.1:4318", "")
			if response.Code != tt.status {
				t.Fatalf("status %d, want %d: %s", response.Code, tt.status, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("sensitive response is cacheable")
			}
		})
	}
	response = request(handler, "GET", base+"/events?revision="+detail.Revision+"&limit=2", "127.0.0.1:4318", "")
	var page inspection.Page[inspection.Record]
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.NextOffset == nil || *page.NextOffset != 2 || page.Total != detail.RecordCount {
		t.Fatalf("bad pagination: %+v", page)
	}
}

func TestReadOnlyAndOriginBoundary(t *testing.T) {
	t.Parallel()
	handler, _, _ := testAPI(t)
	tests := []struct {
		name   string
		method string
		host   string
		origin string
		status int
	}{
		{name: "same origin", method: "GET", host: "127.0.0.1:4318", origin: "http://127.0.0.1:4318", status: 200},
		{name: "localhost", method: "GET", host: "localhost:4318", status: 200},
		{name: "rebind host", method: "GET", host: "attacker.example:4318", status: 403},
		{name: "wrong port", method: "GET", host: "localhost:9999", status: 403},
		{name: "external origin", method: "GET", host: "127.0.0.1:4318", origin: "https://attacker.example", status: 403},
		{name: "opaque origin", method: "GET", host: "127.0.0.1:4318", origin: "null", status: 403},
		{name: "post", method: "POST", host: "127.0.0.1:4318", status: 405},
		{name: "delete", method: "DELETE", host: "127.0.0.1:4318", status: 405},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := request(handler, tt.method, "/api/v1/system", tt.host, tt.origin)
			if response.Code != tt.status {
				t.Fatalf("status %d, want %d", response.Code, tt.status)
			}
			if response.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("CORS was enabled")
			}
		})
	}
	response := request(handler, "GET", "/api/v1/sessions?limit=1", "127.0.0.1:4318", "")
	if strings.Contains(response.Body.String(), "apiKey") {
		t.Fatal("API exposed credential fields")
	}
}
