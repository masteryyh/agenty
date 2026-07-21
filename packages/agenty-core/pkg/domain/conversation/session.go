package conversation

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)
var (
	ErrModelNotConfigured = errors.New("conversation: model is not configured")
	ErrRoundNotFound      = errors.New("conversation: round not found")
	ErrRoundNotRunning    = errors.New("conversation: round is not running")
	ErrInvalidRole        = errors.New("conversation: invalid message role")
)

type Session struct {
	ID                    uuid.UUID             `json:"id"`
	AgentSlug             shared.Slug           `json:"agentSlug"`
	Title                 *string               `json:"title,omitempty"`
	Cwd                   *string               `json:"cwd,omitempty"`
	CurrentModel          *shared.ModelRef      `json:"currentModel,omitempty"`
	ContextWindow         int64                 `json:"contextWindow"`
	CurrentThinkingEffort shared.ThinkingEffort `json:"currentThinkingEffort,omitempty"`
	Rounds                []Round               `json:"rounds"`
	CreatedAt             time.Time             `json:"createdAt"`
	UpdatedAt             time.Time             `json:"updatedAt"`

	pending []shared.Event
}

func StartSession(agentSlug shared.Slug, model shared.ModelRef, contextWindow int64, effort shared.ThinkingEffort, cwd *string) *Session {
	s := &Session{}
	s.record(SessionStarted{
		SessionID:      shared.NewID(),
		Agent:          agentSlug,
		Model:          model,
		ContextWindow:  contextWindow,
		ThinkingEffort: effort,
		Cwd:            cloneString(cwd),
		At:             now(),
	})
	return s
}

func (s *Session) SetModel(model shared.ModelRef, contextWindow int64) {
	s.record(SessionModelSet{
		SessionID:     s.ID,
		Model:         model,
		ContextWindow: contextWindow,
		At:            now(),
	})
}

func (s *Session) SetThinkingEffort(effort shared.ThinkingEffort) {
	s.record(SessionThinkingEffortSet{
		SessionID:      s.ID,
		ThinkingEffort: effort,
		At:             now(),
	})
}

func (s *Session) SetCwd(cwd *string) {
	s.record(SessionCwdSet{SessionID: s.ID, Cwd: cloneString(cwd), At: now()})
}

func (s *Session) StartRound() (uuid.UUID, error) {
	if s.CurrentModel == nil {
		return uuid.Nil, ErrModelNotConfigured
	}

	id := shared.NewID()
	s.record(RoundStarted{
		SessionID:      s.ID,
		RoundID:        id,
		Sequence:       len(s.Rounds) + 1,
		Model:          *s.CurrentModel,
		ContextWindow:  s.ContextWindow,
		ThinkingEffort: s.CurrentThinkingEffort,
		Cwd:            s.Cwd,
		At:             now(),
	})
	return id, nil
}

func (s *Session) AppendMessage(roundID uuid.UUID, role Role, content Content, model *shared.ModelRef, usage *TokenUsage) (Message, error) {
	if !role.Valid() {
		return Message{}, ErrInvalidRole
	}

	r, _, ok := s.findRound(roundID)
	if !ok {
		return Message{}, ErrRoundNotFound
	}
	if r.Status != RoundRunning {
		return Message{}, ErrRoundNotRunning
	}

	msg := Message{
		ID:        shared.NewID(),
		RoundID:   roundID,
		Role:      role,
		Content:   content,
		Model:     model,
		Usage:     usage,
		CreatedAt: now(),
	}
	s.record(MessageAppended{
		SessionID: s.ID,
		Message: msg,
		At: msg.CreatedAt,
	})
	return msg, nil
}

func (s *Session) AppendUserMessage(roundID uuid.UUID, content Content) (Message, error) {
	return s.AppendMessage(roundID, RoleUser, content, nil, nil)
}

func (s *Session) AppendAssistantMessage(roundID uuid.UUID, content Content, model shared.ModelRef, usage *TokenUsage) (Message, error) {
	return s.AppendMessage(roundID, RoleAssistant, content, &model, usage)
}

func (s *Session) CompleteRound(roundID uuid.UUID, status RoundStatus, usage TokenUsage, errMsg *string) error {
	if !status.Terminal() {
		return fmt.Errorf("conversation: %q is not a terminal round status", status)
	}

	r, _, ok := s.findRound(roundID)
	if !ok {
		return ErrRoundNotFound
	}
	if r.Status.Terminal() {
		return ErrRoundNotRunning
	}

	s.record(RoundEnded{
		SessionID: s.ID,
		RoundID:   roundID,
		Status:    status,
		Usage:     usage,
		Error:     errMsg,
		At:        now(),
	})
	return nil
}

func (s *Session) SetTitle(title string) {
	s.record(SessionTitleSet{SessionID: s.ID, Title: title, At: now()})
}

func (s *Session) PendingEvents() []shared.Event {
	return s.pending
}

func (s *Session) ClearPending() {
	s.pending = nil
}

func ReplaySession(events []shared.Event) *Session {
	s := &Session{}
	for _, e := range events {
		s.apply(e)
	}
	return s
}

func (s *Session) record(e shared.Event) {
	s.apply(e)
	s.pending = append(s.pending, e)
}

func (s *Session) apply(e shared.Event) {
	switch ev := e.(type) {
	case SessionStarted:
		s.ID = ev.SessionID
		s.AgentSlug = ev.Agent
		s.CurrentModel = &ev.Model
		s.ContextWindow = ev.ContextWindow
		s.CurrentThinkingEffort = ev.ThinkingEffort
		s.Cwd = cloneString(ev.Cwd)
		s.CreatedAt = ev.At
		s.UpdatedAt = ev.At
	case SessionModelSet:
		s.CurrentModel = &ev.Model
		s.ContextWindow = ev.ContextWindow
		s.UpdatedAt = ev.At
	case SessionThinkingEffortSet:
		s.CurrentThinkingEffort = ev.ThinkingEffort
		s.UpdatedAt = ev.At
	case SessionCwdSet:
		s.Cwd = cloneString(ev.Cwd)
		s.UpdatedAt = ev.At
	case RoundStarted:
		s.Rounds = append(s.Rounds, Round{
			ID:        ev.RoundID,
			SessionID: ev.SessionID,
			Sequence:  ev.Sequence,
			Status:    RoundRunning,
			Model:     ev.Model,
			Cwd:       cloneString(ev.Cwd),
			Messages:  []Message{},
			StartedAt: ev.At,
		})
		s.UpdatedAt = ev.At
	case MessageAppended:
		if r, _, ok := s.findRound(ev.Message.RoundID); ok {
			r.Messages = append(r.Messages, ev.Message)
		}
		s.UpdatedAt = ev.At
	case RoundEnded:
		if r, _, ok := s.findRound(ev.RoundID); ok {
			r.Status = ev.Status
			r.Error = ev.Error
			ended := ev.At
			r.EndedAt = &ended
		}
		s.UpdatedAt = ev.At
	case SessionTitleSet:
		title := ev.Title
		s.Title = &title
		s.UpdatedAt = ev.At
	}
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (s *Session) findRound(id uuid.UUID) (*Round, int, bool) {
	for i := range s.Rounds {
		if s.Rounds[i].ID == id {
			return &s.Rounds[i], i, true
		}
	}
	return nil, -1, false
}

func now() time.Time {
	return time.Now().UTC()
}
