package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-core/pkg/infra/storage"
)

// SessionService implements session CRUD and configuration use-cases on top of
// a ConversationRepository. Sessions are event-sourced: every mutation loads
// the aggregate, applies a domain method that records pending events, saves
// (appending events to the transcript and refreshing the projection), then
// clears the pending buffer.
type SessionService struct {
	repo sessionRepository
}

type sessionRepository interface {
	Load(ctx context.Context, id uuid.UUID) (*conversation.Session, error)
	Save(ctx context.Context, session *conversation.Session) error
	List(ctx context.Context, query conversation.ListQuery) ([]conversation.SessionSummary, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

func NewSessionService(repo sessionRepository) *SessionService {
	return &SessionService{repo: repo}
}

// SessionCreateInput carries the fields for starting a session. AgentSlug and
// the provider/model pair are required so a session always begins with a
// configured model (the conversation domain requires one to start a round).
type SessionCreateInput struct {
	AgentSlug      string                `json:"agentSlug"`
	ProviderSlug   string                `json:"providerSlug"`
	ModelSlug      string                `json:"modelSlug"`
	ContextWindow  int64                 `json:"contextWindow,omitempty"`
	ThinkingEffort shared.ThinkingEffort `json:"thinkingEffort,omitempty"`
	Cwd            *string               `json:"cwd,omitempty"`
}

func (s *SessionService) Create(ctx context.Context, in SessionCreateInput) (*conversation.Session, error) {
	agentSlug, err := shared.NewSlug(in.AgentSlug)
	if err != nil {
		return nil, Validation(err.Error())
	}
	providerSlug, err := shared.NewSlug(in.ProviderSlug)
	if err != nil {
		return nil, Validation(err.Error())
	}
	modelSlug, err := shared.NewSlug(in.ModelSlug)
	if err != nil {
		return nil, Validation(err.Error())
	}

	effort := in.ThinkingEffort
	if effort == "" {
		effort = shared.ThinkingOff
	}
	if !effort.Valid() {
		return nil, Validation("invalid thinking effort: " + string(effort))
	}

	session := conversation.StartSession(
		agentSlug,
		shared.NewModelRef(providerSlug, modelSlug),
		in.ContextWindow,
		effort,
		in.Cwd,
	)
	if err := s.repo.Save(ctx, session); err != nil {
		return nil, Internal("failed to save session: " + err.Error())
	}
	session.ClearPending()
	return session, nil
}

func (s *SessionService) Get(ctx context.Context, idStr string) (*conversation.Session, error) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, Validation("invalid session id: " + err.Error())
	}
	sess, err := s.repo.Load(ctx, id)
	if err != nil {
		if errors.Is(err, storage.ErrConversationNotFound) {
			return nil, NotFound("session " + idStr + " not found")
		}
		return nil, Internal("failed to load session: " + err.Error())
	}
	return sess, nil
}

// SessionListQuery filters and paginates the session listing.
type SessionListQuery struct {
	AgentSlug string
	Limit     int
	Offset    int
}

func (s *SessionService) List(ctx context.Context, q SessionListQuery) ([]conversation.SessionSummary, error) {
	var agentSlug *shared.Slug
	if q.AgentSlug != "" {
		sv, err := shared.NewSlug(q.AgentSlug)
		if err != nil {
			return nil, Validation(err.Error())
		}
		agentSlug = &sv
	}
	sums, err := s.repo.List(ctx, conversation.ListQuery{AgentSlug: agentSlug, Limit: q.Limit, Offset: q.Offset})
	if err != nil {
		return nil, Internal("failed to list sessions: " + err.Error())
	}
	return sums, nil
}

func (s *SessionService) Delete(ctx context.Context, idStr string) error {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return Validation("invalid session id: " + err.Error())
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, storage.ErrConversationNotFound) {
			return NotFound("session " + idStr + " not found")
		}
		return Internal("failed to delete session: " + err.Error())
	}
	return nil
}

func (s *SessionService) SetTitle(ctx context.Context, idStr, title string) (*conversation.Session, error) {
	sess, err := s.loadForUpdate(ctx, idStr)
	if err != nil {
		return nil, err
	}
	sess.SetTitle(title)
	return s.saveUpdated(ctx, sess)
}

func (s *SessionService) SetModel(ctx context.Context, idStr, providerSlug, modelSlug string, contextWindow int64) (*conversation.Session, error) {
	sess, err := s.loadForUpdate(ctx, idStr)
	if err != nil {
		return nil, err
	}
	ps, err := shared.NewSlug(providerSlug)
	if err != nil {
		return nil, Validation(err.Error())
	}
	ms, err := shared.NewSlug(modelSlug)
	if err != nil {
		return nil, Validation(err.Error())
	}
	sess.SetModel(shared.NewModelRef(ps, ms), contextWindow)
	return s.saveUpdated(ctx, sess)
}

func (s *SessionService) SetThinkingEffort(ctx context.Context, idStr string, effort shared.ThinkingEffort) (*conversation.Session, error) {
	sess, err := s.loadForUpdate(ctx, idStr)
	if err != nil {
		return nil, err
	}
	if !effort.Valid() {
		return nil, Validation("invalid thinking effort: " + string(effort))
	}
	sess.SetThinkingEffort(effort)
	return s.saveUpdated(ctx, sess)
}

// SetCwd sets or clears the session working directory. A nil cwd clears it.
func (s *SessionService) SetCwd(ctx context.Context, idStr string, cwd *string) (*conversation.Session, error) {
	sess, err := s.loadForUpdate(ctx, idStr)
	if err != nil {
		return nil, err
	}
	sess.SetCwd(cwd)
	return s.saveUpdated(ctx, sess)
}

func (s *SessionService) loadForUpdate(ctx context.Context, idStr string) (*conversation.Session, error) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, Validation("invalid session id: " + err.Error())
	}
	sess, err := s.repo.Load(ctx, id)
	if err != nil {
		if errors.Is(err, storage.ErrConversationNotFound) {
			return nil, NotFound("session " + idStr + " not found")
		}
		return nil, Internal("failed to load session: " + err.Error())
	}
	return sess, nil
}

func (s *SessionService) saveUpdated(ctx context.Context, sess *conversation.Session) (*conversation.Session, error) {
	if err := s.repo.Save(ctx, sess); err != nil {
		return nil, Internal("failed to save session: " + err.Error())
	}
	sess.ClearPending()
	return sess, nil
}
