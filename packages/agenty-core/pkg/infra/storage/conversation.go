package storage

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

var (
	ErrConversationNotFound = errors.New("storage: session not found")

	errTranscriptNotFound = errors.New("storage: transcript miss")
)

type ConversationRepository struct {
	db          *sql.DB
	sessionsDir string
}

func NewConversationRepository(db *sql.DB, sessionsDir string) *ConversationRepository {
	return &ConversationRepository{db: db, sessionsDir: sessionsDir}
}

func (r *ConversationRepository) Load(ctx context.Context, id uuid.UUID) (*conversation.Session, error) {
	sum, err := r.getSession(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	events, err := r.loadTranscript(id, sum.CreatedAt)
	if err == errTranscriptNotFound {
		return nil, ErrConversationNotFound
	}
	if err != nil {
		return nil, err
	}

	return conversation.ReplaySession(events), nil
}

func (r *ConversationRepository) Save(ctx context.Context, session *conversation.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	pending := session.PendingEvents()
	if len(pending) == 0 {
		return nil
	}

	exists, err := r.transcriptExists(session.ID, session.CreatedAt)
	if err != nil {
		return err
	}

	var seq int64 = 1
	if exists {
		existingEvents, err := r.loadTranscript(session.ID, session.CreatedAt)
		if err != nil {
			return err
		}
		seq = int64(len(existingEvents)) + 1
	}

	if err := r.appendTranscript(session.ID, session.CreatedAt, seq, pending); err != nil {
		return err
	}

	sum := session.Summary()
	if err := r.upsertSession(ctx, sum); err != nil {
		return err
	}

	return nil
}

func (r *ConversationRepository) List(ctx context.Context, query conversation.ListQuery) ([]conversation.SessionSummary, error) {
	return r.listSessions(ctx, query)
}

func (r *ConversationRepository) Delete(ctx context.Context, id uuid.UUID) error {
	sum, err := r.getSession(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConversationNotFound
		}
		return err
	}

	if err := r.deleteTranscript(id, sum.CreatedAt); err != nil {
		return err
	}
	return r.deleteSession(ctx, id)
}

func (r *ConversationRepository) upsertSession(ctx context.Context, sum conversation.SessionSummary) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO sessions (id, title, agent_slug, last_provider_slug, last_model_slug, context_window, last_thinking_effort, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			agent_slug = excluded.agent_slug,
			last_provider_slug = excluded.last_provider_slug,
			last_model_slug = excluded.last_model_slug,
			context_window = excluded.context_window,
			last_thinking_effort = excluded.last_thinking_effort,
			updated_at = excluded.updated_at
	`,
		sum.ID.String(),
		sum.Title,
		sum.AgentSlug.String(),
		sum.LastProviderSlug.String(),
		sum.LastModelSlug.String(),
		sum.ContextWindow,
		sum.LastThinkingEffort,
		sum.CreatedAt.Format(time.RFC3339),
		sum.UpdatedAt.Format(time.RFC3339),
	)
	return err
}

func (r *ConversationRepository) getSession(ctx context.Context, id uuid.UUID) (conversation.SessionSummary, error) {
	var sum conversation.SessionSummary
	var idStr, agentStr, providerStr, modelStr, effortStr, createdStr, updatedStr string

	err := r.db.QueryRowContext(ctx, `
		SELECT id, title, agent_slug, last_provider_slug, last_model_slug, context_window, last_thinking_effort, created_at, updated_at
		FROM sessions WHERE id = ?
	`, id.String()).Scan(&idStr, &sum.Title, &agentStr, &providerStr, &modelStr, &sum.ContextWindow, &effortStr, &createdStr, &updatedStr)

	if err != nil {
		return conversation.SessionSummary{}, err
	}

	if sum.ID, err = uuid.Parse(idStr); err != nil {
		return conversation.SessionSummary{}, err
	}
	if sum.AgentSlug, err = shared.NewSlug(agentStr); err != nil {
		return conversation.SessionSummary{}, err
	}
	if providerStr != "" {
		if sum.LastProviderSlug, err = shared.NewSlug(providerStr); err != nil {
			return conversation.SessionSummary{}, err
		}
	}
	if modelStr != "" {
		if sum.LastModelSlug, err = shared.NewSlug(modelStr); err != nil {
			return conversation.SessionSummary{}, err
		}
	}
	sum.LastThinkingEffort = shared.ThinkingEffort(effortStr)
	if sum.CreatedAt, err = time.Parse(time.RFC3339, createdStr); err != nil {
		return conversation.SessionSummary{}, err
	}
	if sum.UpdatedAt, err = time.Parse(time.RFC3339, updatedStr); err != nil {
		return conversation.SessionSummary{}, err
	}

	return sum, nil
}

func (r *ConversationRepository) listSessions(ctx context.Context, query conversation.ListQuery) ([]conversation.SessionSummary, error) {
	q := "SELECT id, title, agent_slug, last_provider_slug, last_model_slug, context_window, last_thinking_effort, created_at, updated_at FROM sessions"
	args := []any{}

	if query.AgentSlug != nil {
		q += " WHERE agent_slug = ?"
		args = append(args, query.AgentSlug.String())
	}

	q += " ORDER BY updated_at DESC"

	if query.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, query.Limit)
	}
	if query.Offset > 0 {
		q += " OFFSET ?"
		args = append(args, query.Offset)
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []conversation.SessionSummary
	for rows.Next() {
		var sum conversation.SessionSummary
		var idStr, agentStr, providerStr, modelStr, effortStr, createdStr, updatedStr string

		if err := rows.Scan(&idStr, &sum.Title, &agentStr, &providerStr, &modelStr, &sum.ContextWindow, &effortStr, &createdStr, &updatedStr); err != nil {
			return nil, err
		}

		if sum.ID, err = uuid.Parse(idStr); err != nil {
			return nil, err
		}
		if sum.AgentSlug, err = shared.NewSlug(agentStr); err != nil {
			return nil, err
		}
		if providerStr != "" {
			if sum.LastProviderSlug, err = shared.NewSlug(providerStr); err != nil {
				return nil, err
			}
		}
		if modelStr != "" {
			if sum.LastModelSlug, err = shared.NewSlug(modelStr); err != nil {
				return nil, err
			}
		}
		sum.LastThinkingEffort = shared.ThinkingEffort(effortStr)
		if sum.CreatedAt, err = time.Parse(time.RFC3339, createdStr); err != nil {
			return nil, err
		}
		if sum.UpdatedAt, err = time.Parse(time.RFC3339, updatedStr); err != nil {
			return nil, err
		}

		results = append(results, sum)
	}
	return results, rows.Err()
}

func (r *ConversationRepository) deleteSession(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", id.String())
	return err
}

func (r *ConversationRepository) appendTranscript(sessionID uuid.UUID, createdAt time.Time, seq int64, events []shared.Event) error {
	if len(events) == 0 {
		return nil
	}

	path := r.pathFor(sessionID, createdAt)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for i, e := range events {
		line, err := shared.EncodeEvent(seq+int64(i), e)
		if err != nil {
			return err
		}
		if _, err := w.Write(line); err != nil {
			return err
		}
		if err := w.WriteByte('\n'); err != nil {
			return err
		}
	}
	return w.Flush()
}

func (r *ConversationRepository) loadTranscript(sessionID uuid.UUID, createdAt time.Time) ([]shared.Event, error) {
	path := r.pathFor(sessionID, createdAt)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errTranscriptNotFound
		}
		return nil, err
	}
	defer f.Close()

	var events []shared.Event
	reader := bufio.NewReader(f)
	lineNo := 0
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) == 0 && readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, readErr
		}
		lineNo++
		line = bytes.TrimSuffix(line, []byte{'\n'})
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(line) == 0 {
			if errors.Is(readErr, io.EOF) {
				break
			}
			continue
		}
		e, err := conversation.DecodeEventLine(line)
		if err != nil {
			return nil, fmt.Errorf("transcript: line %d: %w", lineNo, err)
		}
		events = append(events, e)
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, readErr
		}
	}
	return events, nil
}

func (r *ConversationRepository) deleteTranscript(sessionID uuid.UUID, createdAt time.Time) error {
	path := r.pathFor(sessionID, createdAt)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (r *ConversationRepository) transcriptExists(sessionID uuid.UUID, createdAt time.Time) (bool, error) {
	path := r.pathFor(sessionID, createdAt)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (r *ConversationRepository) pathFor(sessionID uuid.UUID, createdAt time.Time) string {
	y, m, d := createdAt.Date()
	return filepath.Join(
		r.sessionsDir,
		fmt.Sprintf("%04d", y),
		fmt.Sprintf("%02d", int(m)),
		fmt.Sprintf("%02d", d),
		sessionID.String()+".jsonl",
	)
}
