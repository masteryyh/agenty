package conversation

import (
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

const (
	maxCompactionUserMessages      = 3
	maxCompactionAssistantMessages = 5

	compactionKindSummary           = "summary"
	compactionKindMetadata          = "metadata"
	compactionKindRetainedUser      = "retained_user"
	compactionKindRetainedAssistant = "retained_assistant"
)

func (s *Session) applyCompaction(event SessionCompacted) {
	users, assistants := retainedCompactionMessages(s.context)
	context := make([]Message, 0, len(users)+len(assistants)+2)
	context = append(context, users...)
	context = append(context, compactionSummaryMessage(event))
	if metadata := s.compactionMetadataMessage(event.At, event.CompactionID); metadata != nil {
		context = append(context, *metadata)
	}
	context = append(context, assistants...)
	s.context = context
}

func retainedCompactionMessages(messages []Message) ([]Message, []Message) {
	users := make([]Message, 0, maxCompactionUserMessages)
	assistants := make([]Message, 0, maxCompactionAssistantMessages)
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.IsHidden() {
			continue
		}

		switch message.Role {
		case RoleUser:
			if len(users) >= maxCompactionUserMessages {
				continue
			}
			if retained, ok := retainedUserMessage(message); ok {
				users = append(users, retained)
			}
		case RoleAssistant:
			if len(assistants) >= maxCompactionAssistantMessages {
				continue
			}
			if retained, ok := retainedAssistantMessage(message); ok {
				assistants = append(assistants, retained)
			}
		}
	}
	slices.Reverse(users)
	slices.Reverse(assistants)
	return users, assistants
}

func retainedUserMessage(message Message) (Message, bool) {
	content := make(Content, 0, len(message.Content))
	for _, block := range message.Content {
		switch block.(type) {
		case ReasoningBlock, ToolResultBlock:
			continue
		default:
			content = append(content, block)
		}
	}
	return retainedMessage(message, content, compactionKindRetainedUser)
}

func retainedAssistantMessage(message Message) (Message, bool) {
	content := make(Content, 0, len(message.Content))
	for _, block := range message.Content {
		switch block.(type) {
		case ReasoningBlock, ToolUseBlock, ShellCallBlock:
			continue
		default:
			content = append(content, block)
		}
	}
	return retainedMessage(message, content, compactionKindRetainedAssistant)
}

func retainedMessage(message Message, content Content, kind string) (Message, bool) {
	if len(content) == 0 {
		return Message{}, false
	}

	retained := cloneMessage(message)
	retained.Content = content
	if retained.Metadata == nil {
		retained.Metadata = shared.Metadata{}
	}
	retained.Metadata["compactionKind"] = kind
	return retained, true
}

func compactionSummaryMessage(event SessionCompacted) Message {
	return Message{
		ID:         event.CompactionID,
		Role:       RoleUser,
		Visibility: MessageHidden,
		Metadata:   shared.Metadata{"compactionKind": compactionKindSummary},
		Content:    Text(event.Summary),
		CreatedAt:  event.At,
	}
}

func (s *Session) compactionMetadataMessage(at time.Time, compactionID uuid.UUID) *Message {
	if s.metadata == nil {
		return nil
	}

	text, err := s.metadata.XML()
	if err != nil {
		return nil
	}
	return &Message{
		ID:         uuid.NewSHA1(compactionID, []byte("metadata")),
		Role:       RoleUser,
		Visibility: MessageHidden,
		Metadata:   shared.Metadata{"compactionKind": compactionKindMetadata},
		Content:    Text(text),
		CreatedAt:  at,
	}
}

func (s *Session) refreshCompactionMetadata() {
	summaryIndex := -1
	var compactionID uuid.UUID
	filtered := make([]Message, 0, len(s.context))
	for _, message := range s.context {
		kind, _ := message.Metadata["compactionKind"].(string)
		if kind == compactionKindMetadata {
			continue
		}
		if kind == compactionKindSummary {
			summaryIndex = len(filtered)
			compactionID = message.ID
		}
		filtered = append(filtered, message)
	}
	if summaryIndex < 0 {
		return
	}

	metadata := s.compactionMetadataMessage(s.UpdatedAt, compactionID)
	if metadata == nil {
		s.context = filtered
		return
	}
	filtered = append(filtered, Message{})
	copy(filtered[summaryIndex+2:], filtered[summaryIndex+1:])
	filtered[summaryIndex+1] = *metadata
	s.context = filtered
}

func (s *Session) updateMetadataModel(model shared.ModelRef) {
	if s.metadata == nil || !s.hasCompactionSummary() {
		return
	}
	s.metadata.Model = model.ModelSlug.String()
	s.metadata.Provider = model.ProviderSlug.String()
}

func (s *Session) updateMetadataReasoningEffort(effort shared.ReasoningEffort) {
	if s.metadata == nil || !s.hasCompactionSummary() {
		return
	}
	s.metadata.ReasoningEffort = string(effort)
}

func (s *Session) updateMetadataCwd(cwd *string) {
	if s.metadata == nil || cwd == nil || !s.hasCompactionSummary() {
		return
	}
	s.metadata.Cwd = *cwd
}

func (s *Session) hasCompactionSummary() bool {
	for _, message := range s.context {
		if kind, _ := message.Metadata["compactionKind"].(string); kind == compactionKindSummary {
			return true
		}
	}
	return false
}
