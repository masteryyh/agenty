package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func mustSlug(s string) shared.Slug {
	slug, err := shared.NewSlug(s)
	if err != nil {
		panic(err)
	}
	return slug
}

func defaultModel() shared.ModelRef {
	return shared.NewModelRef("anthropic", "claude-opus")
}

// newConversationRepo opens an isolated SQLite database (bypassing the global
// singleton) for testing the projection + transcript together.
func newConversationRepo(t *testing.T) *ConversationRepository {
	t.Helper()
	tmpDir := t.TempDir()

	db, err := sql.Open("sqlite3", filepath.Join(tmpDir, "test.db")+"?_journal_mode=WAL&_timeout=5000")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		t.Fatalf("exec schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return NewConversationRepository(db, filepath.Join(tmpDir, "sessions"))
}

// newTranscriptOnlyRepo returns a repository whose db is nil, for testing the
// JSONL transcript logic in isolation.
func newTranscriptOnlyRepo(t *testing.T) *ConversationRepository {
	t.Helper()
	return NewConversationRepository(nil, filepath.Join(t.TempDir(), "sessions"))
}

// ---- projection (SQLite sessions table) ----

func TestProjectionUpsertAndGet(t *testing.T) {
	repo := newConversationRepo(t)

	sum := conversation.SessionSummary{
		ID:                 shared.NewID(),
		Title:              "test session",
		AgentSlug:          mustSlug("coder"),
		LastProviderSlug:   mustSlug("anthropic"),
		LastModelSlug:      mustSlug("claude-opus"),
		ContextWindow:      1024,
		LastThinkingEffort: shared.ThinkingHigh,
		CreatedAt:          time.Now().UTC().Truncate(time.Second),
		UpdatedAt:          time.Now().UTC().Truncate(time.Second),
	}

	ctx := context.Background()
	if err := repo.upsertSession(ctx, sum); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.getSession(ctx, sum.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.ID != sum.ID {
		t.Errorf("ID = %v, want %v", got.ID, sum.ID)
	}
	if got.Title != sum.Title {
		t.Errorf("Title = %q, want %q", got.Title, sum.Title)
	}
	if got.AgentSlug != sum.AgentSlug {
		t.Errorf("AgentSlug = %q, want %q", got.AgentSlug, sum.AgentSlug)
	}
	if got.ContextWindow != sum.ContextWindow {
		t.Errorf("ContextWindow = %d, want %d", got.ContextWindow, sum.ContextWindow)
	}
	if got.LastThinkingEffort != sum.LastThinkingEffort {
		t.Errorf("LastThinkingEffort = %q, want %q", got.LastThinkingEffort, sum.LastThinkingEffort)
	}
}

func TestProjectionUpsertUpdatesExisting(t *testing.T) {
	repo := newConversationRepo(t)

	sum := conversation.SessionSummary{
		ID:        shared.NewID(),
		Title:     "original",
		AgentSlug: mustSlug("coder"),
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		UpdatedAt: time.Now().UTC().Truncate(time.Second),
	}

	ctx := context.Background()
	if err := repo.upsertSession(ctx, sum); err != nil {
		t.Fatal(err)
	}

	sum.Title = "updated"
	sum.ContextWindow = 2048
	sum.UpdatedAt = time.Now().UTC().Truncate(time.Second)

	if err := repo.upsertSession(ctx, sum); err != nil {
		t.Fatal(err)
	}

	got, err := repo.getSession(ctx, sum.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "updated" {
		t.Errorf("Title = %q, want updated", got.Title)
	}
	if got.ContextWindow != 2048 {
		t.Errorf("ContextWindow = %d, want 2048", got.ContextWindow)
	}
}

func TestProjectionGetReturnsNotFound(t *testing.T) {
	repo := newConversationRepo(t)

	_, err := repo.getSession(context.Background(), shared.NewID())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("getSession() = %v, want ErrNoRows", err)
	}
}

func TestProjectionList(t *testing.T) {
	repo := newConversationRepo(t)
	ctx := context.Background()
	agentA := mustSlug("agent-a")
	agentB := mustSlug("agent-b")

	baseTime := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		agent := agentA
		if i%2 == 0 {
			agent = agentB
		}
		sum := conversation.SessionSummary{
			ID:        shared.NewID(),
			Title:     "session",
			AgentSlug: agent,
			CreatedAt: baseTime,
			UpdatedAt: baseTime.Add(time.Duration(i) * time.Second),
		}
		if err := repo.upsertSession(ctx, sum); err != nil {
			t.Fatal(err)
		}
	}

	all, err := repo.listSessions(ctx, conversation.ListQuery{})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 5 {
		t.Errorf("List all returned %d, want 5", len(all))
	}

	filtered, err := repo.listSessions(ctx, conversation.ListQuery{AgentSlug: &agentA})
	if err != nil {
		t.Fatalf("List filtered: %v", err)
	}
	if len(filtered) != 2 {
		t.Errorf("List filtered returned %d, want 2", len(filtered))
	}
	for _, s := range filtered {
		if s.AgentSlug != agentA {
			t.Errorf("expected only agent-a, got %s", s.AgentSlug)
		}
	}

	limited, err := repo.listSessions(ctx, conversation.ListQuery{Limit: 2})
	if err != nil {
		t.Fatalf("List limited: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("List limited returned %d, want 2", len(limited))
	}
	if limited[0].UpdatedAt.Before(limited[1].UpdatedAt) {
		t.Errorf("results are not sorted by updatedAt descending: %v then %v", limited[0].UpdatedAt, limited[1].UpdatedAt)
	}

	offset, err := repo.listSessions(ctx, conversation.ListQuery{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("List offset: %v", err)
	}
	if len(offset) != 2 || offset[0].UpdatedAt != all[2].UpdatedAt || offset[1].UpdatedAt != all[3].UpdatedAt {
		t.Errorf("offset results = %+v, want rows 2 and 3", offset)
	}
}

func TestProjectionDelete(t *testing.T) {
	repo := newConversationRepo(t)
	ctx := context.Background()
	sum := conversation.SessionSummary{
		ID:        shared.NewID(),
		AgentSlug: mustSlug("coder"),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := repo.upsertSession(ctx, sum); err != nil {
		t.Fatal(err)
	}
	if err := repo.deleteSession(ctx, sum.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.getSession(ctx, sum.ID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Get after Delete = %v, want ErrNoRows", err)
	}
}

// ---- transcript (per-session JSONL event log) ----

func TestTranscriptAppendAndLoad(t *testing.T) {
	repo := newTranscriptOnlyRepo(t)

	sessionID := shared.NewID()
	createdAt := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	agentSlug := mustSlug("coder")

	events := []shared.Event{
		conversation.SessionStarted{SessionID: sessionID, Agent: agentSlug, Model: shared.NewModelRef("anthropic", "claude-opus"), ContextWindow: 200_000, ThinkingEffort: shared.ThinkingOff, At: createdAt},
		conversation.RoundStarted{SessionID: sessionID, RoundID: shared.NewID(), Sequence: 1, Model: shared.NewModelRef("anthropic", "claude-opus"), ContextWindow: 200_000, ThinkingEffort: shared.ThinkingOff, At: createdAt},
	}

	if err := repo.appendTranscript(sessionID, createdAt, 1, events); err != nil {
		t.Fatalf("Append: %v", err)
	}

	loaded, err := repo.loadTranscript(sessionID, createdAt)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("loaded %d events, want 2", len(loaded))
	}

	if loaded[0].EventType() != conversation.EventSessionStarted {
		t.Errorf("event 0 type = %s, want session_started", loaded[0].EventType())
	}
	if loaded[1].EventType() != conversation.EventRoundStarted {
		t.Errorf("event 1 type = %s, want round_started", loaded[1].EventType())
	}
}

func TestTranscriptAppendIsAppendOnly(t *testing.T) {
	repo := newTranscriptOnlyRepo(t)

	sessionID := shared.NewID()
	createdAt := time.Now().UTC()
	agentSlug := mustSlug("coder")

	first := []shared.Event{
		conversation.SessionStarted{SessionID: sessionID, Agent: agentSlug, Model: shared.NewModelRef("anthropic", "claude-opus"), ContextWindow: 200_000, ThinkingEffort: shared.ThinkingOff, At: createdAt},
	}
	second := []shared.Event{
		conversation.RoundStarted{SessionID: sessionID, RoundID: shared.NewID(), Sequence: 1, Model: shared.NewModelRef("anthropic", "claude-opus"), ContextWindow: 200_000, ThinkingEffort: shared.ThinkingOff, At: createdAt},
	}

	if err := repo.appendTranscript(sessionID, createdAt, 1, first); err != nil {
		t.Fatal(err)
	}
	if err := repo.appendTranscript(sessionID, createdAt, 2, second); err != nil {
		t.Fatal(err)
	}

	loaded, err := repo.loadTranscript(sessionID, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded %d events, want 2", len(loaded))
	}
}

func TestTranscriptLoadsLargeMessage(t *testing.T) {
	t.Parallel()

	repo := newTranscriptOnlyRepo(t)
	sessionID := shared.NewID()
	roundID := shared.NewID()
	createdAt := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	largeText := strings.Repeat("x", 128*1024)
	events := []shared.Event{
		conversation.SessionStarted{SessionID: sessionID, Agent: mustSlug("coder"), Model: defaultModel(), At: createdAt},
		conversation.RoundStarted{SessionID: sessionID, RoundID: roundID, Sequence: 1, Model: defaultModel(), At: createdAt},
		conversation.MessageAppended{SessionID: sessionID, Message: conversation.Message{ID: shared.NewID(), RoundID: roundID, Role: conversation.RoleUser, Content: conversation.Text(largeText), CreatedAt: createdAt}, At: createdAt},
	}
	if err := repo.appendTranscript(sessionID, createdAt, 1, events); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.loadTranscript(sessionID, createdAt)
	if err != nil {
		t.Fatalf("loadTranscript: %v", err)
	}
	messageEvent, ok := loaded[2].(conversation.MessageAppended)
	if !ok {
		t.Fatalf("event 2 = %T, want MessageAppended", loaded[2])
	}
	block, ok := messageEvent.Message.Content[0].(conversation.TextBlock)
	if !ok || block.Text != largeText {
		t.Errorf("large message did not round-trip: type %T, length %d", messageEvent.Message.Content[0], len(block.Text))
	}
}

func TestTranscriptReportsCorruptLine(t *testing.T) {
	t.Parallel()

	repo := newTranscriptOnlyRepo(t)
	sessionID := shared.NewID()
	createdAt := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	path := repo.pathFor(sessionID, createdAt)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	valid, err := shared.EncodeEvent(1, conversation.SessionStarted{SessionID: sessionID, Agent: mustSlug("coder"), Model: defaultModel(), At: createdAt})
	if err != nil {
		t.Fatal(err)
	}
	data := append(append(valid, '\n'), []byte("{broken\n")...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = repo.loadTranscript(sessionID, createdAt)
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("loadTranscript error = %v, want line 2 context", err)
	}
}

func TestTranscriptLoadReturnsNotFoundWhenMissing(t *testing.T) {
	repo := newTranscriptOnlyRepo(t)

	_, err := repo.loadTranscript(shared.NewID(), time.Now().UTC())
	if err != errTranscriptNotFound {
		t.Errorf("loadTranscript() = %v, want errTranscriptNotFound", err)
	}
}

func TestTranscriptDelete(t *testing.T) {
	repo := newTranscriptOnlyRepo(t)

	sessionID := shared.NewID()
	createdAt := time.Now().UTC()
	events := []shared.Event{
		conversation.SessionStarted{SessionID: sessionID, Agent: mustSlug("coder"), Model: defaultModel(), ContextWindow: 200_000, ThinkingEffort: shared.ThinkingOff, At: createdAt},
	}

	if err := repo.appendTranscript(sessionID, createdAt, 1, events); err != nil {
		t.Fatal(err)
	}

	exists, err := repo.transcriptExists(sessionID, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected transcript to exist before delete")
	}

	if err := repo.deleteTranscript(sessionID, createdAt); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	exists, err = repo.transcriptExists(sessionID, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("expected transcript to not exist after delete")
	}
}

func TestTranscriptPathForOrganizesByDate(t *testing.T) {
	repo := newTranscriptOnlyRepo(t)
	sessionID := shared.NewID()
	createdAt := time.Date(2026, 7, 20, 15, 30, 0, 0, time.UTC)

	path := repo.pathFor(sessionID, createdAt)
	expected := filepath.Join(repo.sessionsDir, "2026", "07", "20", sessionID.String()+".jsonl")

	if path != expected {
		t.Errorf("pathFor = %s, want %s", path, expected)
	}
}

// ---- conversation repository (end-to-end) ----

func TestConversationSaveAndLoad(t *testing.T) {
	repo := newConversationRepo(t)
	ctx := context.Background()

	// Start a session, add a round and messages.
	session := conversation.StartSession(mustSlug("coder"), defaultModel(), 200_000, shared.ThinkingOff, nil)
	roundID, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendUserMessage(roundID, conversation.Text("hello")); err != nil {
		t.Fatal(err)
	}
	modelRef := shared.NewModelRef("anthropic", "claude-opus")
	if _, err := session.AppendAssistantMessage(roundID, conversation.Text("hi there"), modelRef, &conversation.TokenUsage{Total: 30}); err != nil {
		t.Fatal(err)
	}
	if err := session.CompleteRound(roundID, conversation.RoundCompleted, conversation.TokenUsage{Total: 30}, nil); err != nil {
		t.Fatal(err)
	}
	session.SetTitle("greeting")

	// Save it.
	if err := repo.Save(ctx, session); err != nil {
		t.Fatalf("Save: %v", err)
	}
	session.ClearPending()

	// Load it back.
	loaded, err := repo.Load(ctx, session.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.ID != session.ID {
		t.Errorf("loaded ID = %v, want %v", loaded.ID, session.ID)
	}
	if len(loaded.Rounds) != 1 {
		t.Fatalf("loaded %d rounds, want 1", len(loaded.Rounds))
	}
	if len(loaded.Rounds[0].Messages) != 2 {
		t.Errorf("loaded %d messages, want 2", len(loaded.Rounds[0].Messages))
	}
	if loaded.Title == nil || *loaded.Title != "greeting" {
		t.Errorf("loaded title = %v, want greeting", loaded.Title)
	}
}

func TestConversationSaveWithCanceledContextHasNoSideEffects(t *testing.T) {
	repo := newConversationRepo(t)
	session := conversation.StartSession(mustSlug("coder"), defaultModel(), 200_000, shared.ThinkingOff, nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := repo.Save(ctx, session); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save error = %v, want context.Canceled", err)
	}
	exists, err := repo.transcriptExists(session.ID, session.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("Save wrote a transcript for an already-canceled context")
	}
	if _, err := repo.Load(t.Context(), session.ID); err != ErrConversationNotFound {
		t.Errorf("Load error = %v, want ErrConversationNotFound", err)
	}
}

func TestConversationSaveAppendsEvents(t *testing.T) {
	repo := newConversationRepo(t)
	ctx := context.Background()

	session := conversation.StartSession(mustSlug("coder"), defaultModel(), 200_000, shared.ThinkingOff, nil)
	if err := repo.Save(ctx, session); err != nil {
		t.Fatal(err)
	}
	session.ClearPending()

	// Add more events and save again.
	roundID, err := session.StartRound()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.AppendUserMessage(roundID, conversation.Text("hi")); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, session); err != nil {
		t.Fatal(err)
	}
	session.ClearPending()

	// Load should see all events.
	loaded, err := repo.Load(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Rounds) != 1 {
		t.Errorf("loaded %d rounds, want 1", len(loaded.Rounds))
	}
}

func TestConversationList(t *testing.T) {
	repo := newConversationRepo(t)
	ctx := context.Background()

	agentA := mustSlug("agent-a")
	agentB := mustSlug("agent-b")

	for i := 0; i < 3; i++ {
		agent := agentA
		if i == 2 {
			agent = agentB
		}
		s := conversation.StartSession(agent, defaultModel(), 200_000, shared.ThinkingOff, nil)
		if err := repo.Save(ctx, s); err != nil {
			t.Fatal(err)
		}
	}

	all, err := repo.List(ctx, conversation.ListQuery{})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("List all returned %d, want 3", len(all))
	}

	filtered, err := repo.List(ctx, conversation.ListQuery{AgentSlug: &agentA})
	if err != nil {
		t.Fatalf("List filtered: %v", err)
	}
	if len(filtered) != 2 {
		t.Errorf("List filtered returned %d, want 2", len(filtered))
	}
}

func TestConversationDelete(t *testing.T) {
	repo := newConversationRepo(t)
	ctx := context.Background()

	session := conversation.StartSession(mustSlug("coder"), defaultModel(), 200_000, shared.ThinkingOff, nil)
	if err := repo.Save(ctx, session); err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(ctx, session.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.Load(ctx, session.ID)
	if err != ErrConversationNotFound {
		t.Errorf("Load after Delete = %v, want ErrConversationNotFound", err)
	}
}

func TestConversationLoadReturnsNotFoundWhenMissing(t *testing.T) {
	repo := newConversationRepo(t)
	_, err := repo.Load(context.Background(), shared.NewID())
	if err != ErrConversationNotFound {
		t.Errorf("Load() = %v, want ErrConversationNotFound", err)
	}
}
