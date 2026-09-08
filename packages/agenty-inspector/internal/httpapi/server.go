package httpapi

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-inspector/internal/inspection"
)

type Options struct {
	Store     *inspection.Store
	Assets    fs.FS
	Address   string
	DevOrigin string
}

type server struct {
	store *inspection.Store
}

func New(options Options) http.Handler {
	api := &server{store: options.Store}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/system", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, api.store.System())
	})
	mux.HandleFunc("GET /api/v1/sessions", api.sessions)
	mux.HandleFunc("GET /api/v1/sessions/{id}", api.detail)
	mux.HandleFunc("GET /api/v1/sessions/{id}/events", api.events)
	mux.HandleFunc("GET /api/v1/sessions/{id}/search", api.events)
	mux.HandleFunc("GET /api/v1/sessions/{id}/records/{recordId}", api.record)
	mux.HandleFunc("GET /api/v1/sessions/{id}/rounds/{roundId}/messages", api.messages)
	mux.HandleFunc("GET /api/v1/sessions/{id}/context", api.context)
	mux.HandleFunc("GET /api/v1/sessions/{id}/diagnostics", api.diagnostics)
	mux.HandleFunc("GET /api/v1/changes", api.changes)
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, r *http.Request) {
		problem(w, http.StatusNotFound, "not_found", "API route not found.")
	})
	mux.Handle("GET /", http.FileServerFS(options.Assets))
	return protect(options, mux)
}

func protect(options Options, next http.Handler) http.Handler {
	_, port, _ := net.SplitHostPort(options.Address)
	hosts := map[string]bool{
		"127.0.0.1:" + port: true,
		"localhost:" + port: true,
		"[::1]:" + port:     true,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		if !hosts[r.Host] {
			problem(w, http.StatusForbidden, "invalid_host", "This service accepts local requests only.")
			return
		}

		if origin := r.Header.Get("Origin"); origin != "" {
			parsed, err := url.Parse(origin)
			sameOrigin := err == nil && parsed.Scheme == "http" && parsed.Host == r.Host
			if !sameOrigin && origin != options.DevOrigin {
				problem(w, http.StatusForbidden, "invalid_origin", "Cross-origin access is disabled.")
				return
			}
		}

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			problem(w, http.StatusMethodNotAllowed, "read_only", "Inspector is read-only.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) sessions(w http.ResponseWriter, r *http.Request) {
	offset, limit, ok := pagination(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	items := s.store.List(q.Get("q"), q.Get("model"), q.Get("since"), q.Get("until"), q.Get("issues") == "true")
	writeJSON(w, http.StatusOK, inspection.Paginate(items, offset, limit, strconv.FormatUint(s.store.System().Generation, 10)))
}

func (s *server) snapshot(w http.ResponseWriter, r *http.Request, requireRevision bool) *inspection.Snapshot {
	revision := r.URL.Query().Get("revision")
	if requireRevision && revision == "" {
		problem(w, http.StatusBadRequest, "revision_required", "Select a session snapshot first.")
		return nil
	}

	snapshot, err := s.store.Snapshot(r.Context(), r.PathValue("id"), revision)
	if err != nil {
		status, code := http.StatusInternalServerError, "read_failed"
		switch {
		case errors.Is(err, inspection.ErrNotFound):
			status, code = http.StatusNotFound, "not_found"
		case errors.Is(err, inspection.ErrSnapshotExpired), errors.Is(err, inspection.ErrChanged):
			status, code = http.StatusConflict, "snapshot_expired"
		case errors.Is(err, inspection.ErrTooLarge):
			status, code = http.StatusRequestEntityTooLarge, "transcript_too_large"
		}
		problem(w, status, code, err.Error())
		return nil
	}
	return snapshot
}

func (s *server) detail(w http.ResponseWriter, r *http.Request) {
	if snapshot := s.snapshot(w, r, false); snapshot != nil {
		writeJSON(w, http.StatusOK, snapshot.Detail)
	}
}

func (s *server) events(w http.ResponseWriter, r *http.Request) {
	offset, limit, ok := pagination(w, r)
	if !ok {
		return
	}

	query := r.URL.Query().Get("q")
	if len(query) > 1024 {
		problem(w, http.StatusBadRequest, "invalid_query", "Search text must be at most 1024 bytes.")
		return
	}

	snapshot := s.snapshot(w, r, true)
	if snapshot == nil {
		return
	}

	records, err := snapshot.Search(r.Context(), query, r.URL.Query().Get("type"), r.URL.Query().Get("roundId"))
	if err != nil {
		return
	}
	writeJSON(w, http.StatusOK, inspection.Paginate(records, offset, limit, snapshot.Detail.Revision))
}

func (s *server) record(w http.ResponseWriter, r *http.Request) {
	snapshot := s.snapshot(w, r, true)
	if snapshot == nil {
		return
	}

	record, ok := snapshot.Record(r.PathValue("recordId"))
	if !ok {
		problem(w, http.StatusNotFound, "not_found", "Record not found in this snapshot.")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *server) messages(w http.ResponseWriter, r *http.Request) {
	offset, limit, ok := pagination(w, r)
	if !ok {
		return
	}

	snapshot := s.snapshot(w, r, true)
	if snapshot == nil {
		return
	}
	writeJSON(w, http.StatusOK, inspection.Paginate(snapshot.Messages(r.PathValue("roundId")), offset, limit, snapshot.Detail.Revision))
}

func (s *server) context(w http.ResponseWriter, r *http.Request) {
	snapshot := s.snapshot(w, r, true)
	if snapshot == nil {
		return
	}

	view, found, err := snapshot.Context(r.Context(), r.URL.Query().Get("atRecord"))
	if err != nil {
		problem(w, http.StatusRequestTimeout, "cancelled", err.Error())
		return
	}

	if !found {
		problem(w, http.StatusNotFound, "not_found", "Choose a record to inspect its context.")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *server) diagnostics(w http.ResponseWriter, r *http.Request) {
	if snapshot := s.snapshot(w, r, true); snapshot != nil {
		writeJSON(w, http.StatusOK, snapshot.Detail.Diagnostics)
	}
}

func (s *server) changes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	control := http.NewResponseController(w)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	previous := ""
	idle := 0
	for {
		current := strconv.FormatUint(s.store.System().Generation, 10)
		if current != previous || idle >= 15 {
			if err := control.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "id: %s\ndata: %s\n\n", current, current); err != nil {
				return
			}
			if err := control.Flush(); err != nil {
				return
			}
			previous, idle = current, 0
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			idle++
		}
	}
}

func pagination(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	parse := func(name string, fallback, maximum int) (int, error) {
		text := r.URL.Query().Get(name)
		if text == "" {
			return fallback, nil
		}
		number, err := strconv.Atoi(text)
		if err != nil || number < 0 || number > maximum {
			return 0, fmt.Errorf("invalid %s", name)
		}
		return number, nil
	}
	offset, err := parse("offset", 0, 10000000)
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid_pagination", err.Error())
		return 0, 0, false
	}
	limit, err := parse("limit", 100, 200)
	if err != nil || limit == 0 {
		problem(w, http.StatusBadRequest, "invalid_pagination", "Limit must be between 1 and 200.")
		return 0, 0, false
	}
	return offset, limit, true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		return // The client disconnected; there is no second response to send.
	}
}

func problem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: strings.TrimSpace(message)})
}
