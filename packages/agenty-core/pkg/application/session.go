package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-core/pkg/infra/storage"
)

type SessionService struct {
	repo           sessionRepository
	executionState sessionExecutionState
}

type sessionExecutionState interface {
	ExecuteSessionIfIdle(sessionID uuid.UUID, execute func() error) (bool, error)
}

type SessionServiceOption func(*SessionService)

func WithSessionExecutionState(state sessionExecutionState) SessionServiceOption {
	return func(service *SessionService) {
		service.executionState = state
	}
}

type sessionRepository interface {
	Load(ctx context.Context, id uuid.UUID) (*conversation.Session, error)
	Save(ctx context.Context, session *conversation.Session) error
	List(ctx context.Context, query conversation.ListQuery) ([]conversation.SessionSummary, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

func NewSessionService(repo sessionRepository, options ...SessionServiceOption) *SessionService {
	service := &SessionService{repo: repo}
	for _, option := range options {
		option(service)
	}

	return service
}

type SessionCreateInput struct {
	AgentSlug       string                 `json:"agentSlug"`
	ProviderSlug    string                 `json:"providerSlug"`
	ModelSlug       string                 `json:"modelSlug"`
	ContextWindow   int64                  `json:"contextWindow,omitempty"`
	ReasoningEffort shared.ReasoningEffort `json:"reasoningEffort,omitempty"`
	Cwd             *string                `json:"cwd,omitempty"`
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

	effort := in.ReasoningEffort
	if effort == "" {
		effort = shared.ReasoningOff
	}
	if !effort.Valid() {
		return nil, Validation("invalid reasoning effort: " + string(effort))
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
	deleteSession := func() error {
		if err := s.repo.Delete(ctx, id); err != nil {
			if errors.Is(err, storage.ErrConversationNotFound) {
				return NotFound("session " + idStr + " not found")
			}
			return Internal("failed to delete session: " + err.Error())
		}

		return nil
	}
	if s.executionState == nil {
		return deleteSession()
	}

	executed, err := s.executionState.ExecuteSessionIfIdle(id, deleteSession)
	if err != nil {
		return err
	}
	if !executed {
		return AlreadyExists("session " + idStr + " is running")
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

func (s *SessionService) SetReasoningEffort(ctx context.Context, idStr string, effort shared.ReasoningEffort) (*conversation.Session, error) {
	sess, err := s.loadForUpdate(ctx, idStr)
	if err != nil {
		return nil, err
	}
	if !effort.Valid() {
		return nil, Validation("invalid reasoning effort: " + string(effort))
	}

	sess.SetReasoningEffort(effort)
	return s.saveUpdated(ctx, sess)
}

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
