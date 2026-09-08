package inspection

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/infra/transcript"
	"github.com/masteryyh/agenty-inspector/internal/testfixture"
)

func writeFixture(t *testing.T, dir string) (string, []byte) {
	t.Helper()
	events := testfixture.Events()
	started := events[0].(conversation.SessionStarted)
	path := transcript.Path(filepath.Join(dir, "sessions"), started.SessionID, started.At)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	raw := testfixture.Encode(events)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path, raw
}

func TestStoreReadsWithoutSQLiteAndKeepsFrozenSnapshots(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path, original := writeFixture(t, dir)
	store := NewStore(dir)
	store.Scan(t.Context())
	entries := store.List("", "", "", "", "", false)
	if len(entries) != 1 || entries[0].Title == "" {
		t.Fatalf("missing session: %+v", entries)
	}
	first, err := store.Snapshot(t.Context(), entries[0].ID, "")
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("{\"type\":"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	store.Scan(t.Context())
	latest, err := store.Snapshot(t.Context(), entries[0].ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if latest.Detail.Complete || latest.Detail.Revision == first.Detail.Revision {
		t.Fatal("append did not create a partial new snapshot")
	}
	pinned, err := store.Snapshot(t.Context(), entries[0].ID, first.Detail.Revision)
	if err != nil || string(pinned.raw) != string(original) {
		t.Fatalf("old snapshot changed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "agenty.sqlite")); !os.IsNotExist(err) {
		t.Fatal("inspector initialized a database")
	}
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	store.Scan(t.Context())
	restored, err := store.Snapshot(t.Context(), entries[0].ID, "")
	if err != nil || restored.Detail.Revision != first.Detail.Revision {
		t.Fatalf("truncation not handled: %v", err)
	}
}

func TestStoreMissingDirectoryAndEscapingSymlink(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "not-created")
	store := NewStore(missing)
	store.Scan(t.Context())
	if store.System().Error == "" {
		t.Fatal("missing path did not produce a diagnostic")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("missing data directory was created")
	}
	dir := t.TempDir()
	external, _ := writeFixture(t, t.TempDir())
	if err := os.Mkdir(filepath.Join(dir, "sessions"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(dir, "sessions", "escape.jsonl")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	store = NewStore(dir)
	store.Scan(t.Context())
	for _, entry := range store.List("", "", "", "", "", false) {
		if snapshot, err := store.Snapshot(t.Context(), entry.ID, ""); err == nil {
			t.Fatalf("read escaped transcript: %+v", snapshot.Detail)
		}
	}
}

func TestStoreConcurrentReadsAreReadOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path, raw := writeFixture(t, dir)
	store := NewStore(dir)
	store.Scan(t.Context())
	entry := store.List("", "", "", "", "", false)[0]
	var group sync.WaitGroup
	for range 5 {
		group.Go(func() {
			store.Scan(t.Context())
			if _, err := store.Snapshot(t.Context(), entry.ID, ""); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(raw) {
		t.Fatalf("source was changed: %v", err)
	}
	if _, err := store.Snapshot(t.Context(), entry.ID, "unknown-revision"); !errors.Is(err, ErrSnapshotExpired) {
		t.Fatalf("invalid revision error = %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	store.Scan(t.Context())
	if _, err := store.Snapshot(t.Context(), entry.ID, entry.Revision); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted source error = %v", err)
	}
}
