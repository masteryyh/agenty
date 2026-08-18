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
	ErrInvalidCompaction  = errors.New("conversation: invalid compaction")
)

type Session struct {
	ID                     uuid.UUID              `json:"id"`
	AgentSlug              shared.Slug            `json:"agentSlug"`
	Title                  *string                `json:"title,omitempty"`
	Cwd                    *string                `json:"cwd,omitempty"`
	CurrentModel           *shared.ModelRef       `json:"currentModel,omitempty"`
	ContextWindow          int64                  `json:"contextWindow"`
	CurrentReasoningEffort shared.ReasoningEffort `json:"currentReasoningEffort,omitempty"`
	Rounds                 []Round                `json:"rounds"`
	CreatedAt              time.Time              `json:"createdAt"`
	UpdatedAt              time.Time              `json:"updatedAt"`

	pending  []shared.Event
	metadata *SessionMetadata
	context  []Message
}

type CompactionInput struct {
	CompactionID        uuid.UUID
	Trigger             CompactionTrigger
	Summary             string
	ContextTokensBefore int64
	Usage               TokenUsage
	At                  time.Time
}

func StartSession(agentSlug shared.Slug, model shared.ModelRef, contextWindow int64, effort shared.ReasoningEffort, cwd *string) *Session {
	s := &Session{Rounds: make([]Round, 0)}
	s.record(SessionStarted{
		SessionID:       shared.NewID(),
		Agent:           agentSlug,
		Model:           model,
		ContextWindow:   contextWindow,
		ReasoningEffort: effort,
		Cwd:             cloneString(cwd),
		At:              now(),
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

func (s *Session) SetReasoningEffort(effort shared.ReasoningEffort) {
	s.record(SessionReasoningEffortSet{
		SessionID:       s.ID,
		ReasoningEffort: effort,
		At:              now(),
	})
}

func (s *Session) SetCwd(cwd *string) {
	s.record(SessionCwdSet{SessionID: s.ID, Cwd: cloneString(cwd), At: now()})
}

func (s *Session) StartRound() (uuid.UUID, error) {
	if s.CurrentModel == nil || s.CurrentModel.IsZero() {
		return uuid.Nil, ErrModelNotConfigured
	}

	id := shared.NewID()
	s.record(RoundStarted{
		SessionID:       s.ID,
		RoundID:         id,
		Sequence:        len(s.Rounds) + 1,
		Model:           *s.CurrentModel,
		ContextWindow:   s.ContextWindow,
		ReasoningEffort: s.CurrentReasoningEffort,
		Cwd:             s.Cwd,
		At:              now(),
	})
	return id, nil
}

func (s *Session) AppendMessage(roundID uuid.UUID, role Role, content Content, model *shared.ModelRef, usage *TokenUsage) (Message, error) {
	return s.appendMessage(roundID, role, content, model, usage, MessageVisible)
}

func (s *Session) appendMessage(
	roundID uuid.UUID,
	role Role,
	content Content,
	model *shared.ModelRef,
	usage *TokenUsage,
	visibility MessageVisibility,
) (Message, error) {
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
		ID:         shared.NewID(),
		RoundID:    roundID,
		Role:       role,
		Visibility: visibility,
		Content:    content,
		Model:      model,
		Usage:      usage,
		CreatedAt:  now(),
	}
	s.record(MessageAppended{
		SessionID: s.ID,
		Message:   msg,
		At:        msg.CreatedAt,
	})
	return msg, nil
}

func (s *Session) AppendUserMessage(roundID uuid.UUID, content Content) (Message, error) {
	return s.AppendMessage(roundID, RoleUser, content, nil, nil)
}

func (s *Session) AppendHiddenUserMessage(roundID uuid.UUID, content Content) (Message, error) {
	return s.appendMessage(roundID, RoleUser, content, nil, nil, MessageHidden)
}

func (s *Session) AppendAssistantMessage(roundID uuid.UUID, content Content, model shared.ModelRef, usage *TokenUsage) (Message, error) {
	return s.AppendMessage(roundID, RoleAssistant, content, &model, usage)
}

// Compact records a compaction event while keeping the append-only rounds
// untouched. The generated synthetic messages are used only when constructing
// the next model request and are recreated from the event during replay.
func (s *Session) Compact(input CompactionInput) (SessionCompacted, error) {
	if input.Trigger != CompactionTriggerManual && input.Trigger != CompactionTriggerAuto && input.Trigger != CompactionTriggerModelSwitch {
		return SessionCompacted{}, ErrInvalidCompaction
	}
	if input.Summary == "" {
		return SessionCompacted{}, ErrInvalidCompaction
	}
	if s.ID == uuid.Nil {
		return SessionCompacted{}, ErrInvalidCompaction
	}

	at := input.At
	if at.IsZero() {
		at = now()
	}
	event := SessionCompacted{
		SessionID:           s.ID,
		CompactionID:        input.CompactionID,
		Trigger:             input.Trigger,
		Summary:             input.Summary,
		ContextTokensBefore: input.ContextTokensBefore,
		Usage:               input.Usage,
		At:                  at,
	}
	if event.CompactionID == uuid.Nil {
		event.CompactionID = shared.NewID()
	}
	s.record(event)
	return event, nil
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

func (s *Session) VisibleCopy() *Session {
	copy := *s
	copy.Rounds = make([]Round, len(s.Rounds))
	if s.metadata != nil {
		metadata := *s.metadata
		copy.metadata = &metadata
	}
	for index, round := range s.Rounds {
		copy.Rounds[index] = round
		copy.Rounds[index].Messages = make([]Message, 0, len(round.Messages))
		for _, message := range round.Messages {
			if message.IsHidden() {
				continue
			}
			copy.Rounds[index].Messages = append(copy.Rounds[index].Messages, message)
		}
	}
	copy.pending = nil
	copy.context = nil
	return &copy
}

func (s *Session) ContextMessages() []Message {
	messages := make([]Message, len(s.context))
	copy(messages, s.context)
	return messages
}

func ReplaySession(events []shared.Event) *Session {
	s := &Session{Rounds: make([]Round, 0)}
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
		s.CurrentReasoningEffort = ev.ReasoningEffort
		s.Cwd = cloneString(ev.Cwd)
		s.Rounds = make([]Round, 0)
		s.context = make([]Message, 0)
		s.CreatedAt = ev.At
		s.UpdatedAt = ev.At
	case SessionModelSet:
		s.CurrentModel = &ev.Model
		s.ContextWindow = ev.ContextWindow
		s.updateMetadataModel(ev.Model)
		s.refreshCompactionMetadata()
		s.UpdatedAt = ev.At
	case SessionReasoningEffortSet:
		s.CurrentReasoningEffort = ev.ReasoningEffort
		s.updateMetadataReasoningEffort(ev.ReasoningEffort)
		s.refreshCompactionMetadata()
		s.UpdatedAt = ev.At
	case SessionCwdSet:
		s.Cwd = cloneString(ev.Cwd)
		s.updateMetadataCwd(ev.Cwd)
		s.refreshCompactionMetadata()
		s.UpdatedAt = ev.At
	case RoundStarted:
		s.Rounds = append(s.Rounds, Round{
			ID:              ev.RoundID,
			SessionID:       ev.SessionID,
			Sequence:        ev.Sequence,
			Status:          RoundRunning,
			Model:           ev.Model,
			ContextWindow:   ev.ContextWindow,
			ReasoningEffort: ev.ReasoningEffort,
			Cwd:             cloneString(ev.Cwd),
			Messages:        []Message{},
			StartedAt:       ev.At,
		})
		s.UpdatedAt = ev.At
	case MessageAppended:
		if r, _, ok := s.findRound(ev.Message.RoundID); ok {
			r.Messages = append(r.Messages, ev.Message)
		}
		s.context = append(s.context, ev.Message)
		s.applyMessageMetadata(ev.Message)
		s.UpdatedAt = ev.At
	case SessionCompacted:
		s.applyCompaction(ev)
		s.UpdatedAt = ev.At
	case SessionMetadataRefreshed:
		s.applyMessageMetadata(ev.Message)
		s.refreshCompactionMetadata()
		s.UpdatedAt = ev.At
	case RoundEnded:
		if r, _, ok := s.findRound(ev.RoundID); ok {
			r.Status = ev.Status
			r.Usage = ev.Usage
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

func cloneMessages(messages []Message) []Message {
	if messages == nil {
		return nil
	}
	cloned := make([]Message, len(messages))
	for index, message := range messages {
		cloned[index] = cloneMessage(message)
	}
	return cloned
}

func cloneMessage(message Message) Message {
	cloned := message
	cloned.Content = append(Content(nil), message.Content...)
	if message.Model != nil {
		model := *message.Model
		cloned.Model = &model
	}
	if message.Usage != nil {
		usage := *message.Usage
		cloned.Usage = &usage
	}
	if message.Metadata != nil {
		cloned.Metadata = make(shared.Metadata, len(message.Metadata))
		for key, value := range message.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return cloned
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
