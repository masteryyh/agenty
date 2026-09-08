package inspection

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	"github.com/masteryyh/agenty-core/pkg/infra/transcript"
)

type Snapshot struct {
	recordIndex  map[string]int
	Detail       Detail
	Records      []Record
	raw          []byte
	events       []shared.Event
	eventRecords []int
	session      *conversation.Session
	messages     map[string][]MessageNode
}

func Build(ctx context.Context, entry SessionEntry, raw []byte) (*Snapshot, error) {
	hash := sha256.Sum256(raw)
	s := &Snapshot{
		raw: raw, Records: []Record{}, events: []shared.Event{}, eventRecords: []int{},
		messages: map[string][]MessageNode{}, recordIndex: map[string]int{},
		Detail: Detail{Entry: entry, Revision: fmt.Sprintf("%x", hash[:16]),
			Rounds: []RoundNode{}, Diagnostics: []Diagnostic{}, Tools: []ToolRelation{}, Complete: true},
	}
	stopped := false
	var previous int64
	err := transcript.Read(ctx, bytes.NewReader(raw), func(line transcript.Record) error {
		record := Record{ID: strconv.FormatInt(line.Offset, 10), Line: line.Line,
			Offset: line.Offset, Length: line.Length, Terminated: line.Terminated, Type: "invalid"}
		if len(line.Bytes) == 0 {
			record.Type = "empty"
			s.Records = append(s.Records, record)
			s.issue("empty_line", "info", "Empty physical line; core skips this line.", record)
			return nil
		}
		if !utf8.Valid(line.Bytes) {
			s.issue("invalid_utf8", "warning", "Record contains invalid UTF-8. Original bytes are available as base64.", record)
		}
		envelope, decodeErr := shared.DecodeEnvelope(line.Bytes)
		if decodeErr == nil {
			record.Seq, record.Type, record.WroteAt = envelope.Seq, envelope.Type, envelope.WroteAt
			if envelope.Seq != previous+1 {
				s.issue("event_sequence", "warning", fmt.Sprintf("Expected seq %d; found %d.", previous+1, envelope.Seq), record)
			}
			previous = envelope.Seq
		}
		var event shared.Event
		if decodeErr == nil {
			event, decodeErr = conversation.DecodeEvent(envelope)
		}
		if decodeErr != nil {
			record.Error = decodeErr.Error()
			code, severity := "decode_error", "error"
			if !line.Terminated {
				code, severity = "trailing_fragment", "warning"
			}
			s.issue(code, severity, record.Error, record)
			stopped, s.Detail.Complete = true, false
		} else {
			record.RoundID, record.Summary = describe(event)
			if !line.Terminated {
				s.issue("unterminated_record", "info", "Valid final record without a newline.", record)
			}
			if !stopped {
				record.Applied = true
				s.events = append(s.events, event)
				s.eventRecords = append(s.eventRecords, len(s.Records))
				s.Detail.ReplayedThrough = record.Line
			}
		}
		s.Records = append(s.Records, record)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for i, record := range s.Records {
		s.recordIndex[record.ID] = i
	}
	s.session = conversation.ReplaySession(s.events)
	s.project()
	s.validate()
	return s, nil
}

func describe(event shared.Event) (string, string) {
	switch e := event.(type) {
	case conversation.MessageAppended:
		return e.Message.RoundID.String(), string(e.Message.Role) + " · " + contentPreview(e.Message.Content)
	case conversation.RoundStarted:
		return e.RoundID.String(), fmt.Sprintf("Round %d · %s", e.Sequence, e.Model.ModelCode)
	case conversation.RoundEnded:
		return e.RoundID.String(), string(e.Status)
	case conversation.SessionCompacted:
		return "", string(e.Trigger) + " · " + preview(e.Summary)
	case conversation.SessionTitleSet:
		return "", e.Title
	case conversation.SessionStarted:
		return "", e.Agent.String() + " · " + e.Model.ModelCode.String()
	case conversation.SessionModelSet:
		return "", e.Model.ProviderCode.String() + " / " + e.Model.ModelCode.String()
	case conversation.SessionCwdSet:
		if e.Cwd != nil {
			return "", *e.Cwd
		}
	case conversation.SessionReasoningEffortSet:
		return "", string(e.ReasoningEffort)
	}
	return "", event.EventType()
}

func preview(text string) string {
	runes := []rune(strings.Join(strings.Fields(text), " "))
	if len(runes) > 160 {
		return string(runes[:160]) + "…"
	}
	return string(runes)
}

func contentPreview(content conversation.Content) string {
	parts := []string{}
	for _, block := range content {
		switch b := block.(type) {
		case conversation.TextBlock:
			parts = append(parts, preview(b.Text))
		case conversation.ToolUseBlock:
			parts = append(parts, b.Name)
		default:
			parts = append(parts, string(block.BlockType()))
		}
		if len(parts) >= 3 {
			break
		}
	}
	return preview(strings.Join(parts, " · "))
}

func ref(record Record) SourceRef {
	return SourceRef{RecordID: record.ID, Line: record.Line, Seq: record.Seq}
}

func (s *Snapshot) issue(code, severity, message string, record Record) {
	s.Detail.Diagnostics = append(s.Detail.Diagnostics, Diagnostic{
		Code: code, Severity: severity, Message: message, Source: ref(record),
	})
}

func (s *Snapshot) project() {
	session := s.session
	s.Detail.SessionID, s.Detail.Title = session.ID.String(), session.Title
	s.Detail.AgentCode, s.Detail.CurrentModel = session.AgentCode, session.CurrentModel
	s.Detail.Cwd, s.Detail.ContextWindow = sessionCwd(session, s.events), session.ContextWindow
	s.Detail.ReasoningEffort = session.CurrentReasoningEffort
	s.Detail.RecordCount = len(s.Records)
	roundSources := map[string]SourceRef{}
	for i, event := range s.events {
		source := ref(s.Records[s.eventRecords[i]])
		switch e := event.(type) {
		case conversation.RoundStarted:
			roundSources[e.RoundID.String()] = source
		case conversation.MessageAppended:
			id := e.Message.RoundID.String()
			s.messages[id] = append(s.messages[id], MessageNode{Message: e.Message, Source: source})
			s.Detail.MessageCount++
			if e.Message.IsHidden() {
				s.Detail.HiddenCount++
			}
		}
	}
	for _, round := range session.Rounds {
		s.Detail.Rounds = append(s.Detail.Rounds, RoundNode{
			ID: round.ID.String(), Sequence: round.Sequence, Status: round.Status, Model: round.Model,
			ReasoningEffort: round.ReasoningEffort, ContextWindow: round.ContextWindow,
			Cwd: round.Cwd, StartedAt: round.StartedAt, EndedAt: round.EndedAt, Usage: round.Usage,
			Error: round.Error, MessageCount: len(round.Messages), Source: roundSources[round.ID.String()],
		})
	}
}

func sessionCwd(session *conversation.Session, events []shared.Event) *string {
	if session.Cwd != nil {
		return session.Cwd
	}

	// A null cwd event is an explicit clear and must win over older metadata.
	for _, event := range events {
		if cwdEvent, ok := event.(conversation.SessionCwdSet); ok && cwdEvent.SessionID == session.ID {
			return nil
		}
	}

	metadata := session.LastMetadata()
	if metadata == nil || strings.TrimSpace(metadata.Cwd) == "" {
		return nil
	}
	cwd := metadata.Cwd
	return &cwd
}

func (s *Snapshot) Messages(roundID string) []MessageNode {
	if messages, ok := s.messages[roundID]; ok {
		return messages
	}
	return []MessageNode{}
}

func (s *Snapshot) Record(id string) (RecordDetail, bool) {
	index, ok := s.recordIndex[id]
	if !ok {
		return RecordDetail{}, false
	}
	record := s.Records[index]
	raw := s.raw[record.Offset : record.Offset+record.Length]
	result := RecordDetail{Record: record, Raw: string(raw)}
	if !utf8.Valid(raw) {
		result.RawBase64 = base64.StdEncoding.EncodeToString(raw)
	}
	if envelope, err := shared.DecodeEnvelope(bytes.TrimSpace(raw)); err == nil {
		result.Envelope = &envelope
	}
	return result, true
}

func (s *Snapshot) Search(ctx context.Context, query, eventType, roundID string) ([]Record, error) {
	records := []Record{}
	query = strings.ToLower(query)
	for _, record := range s.Records {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if eventType != "" && record.Type != eventType {
			continue
		}
		if roundID != "" && record.RoundID != roundID {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(string(s.raw[record.Offset:record.Offset+record.Length])), query) {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func (s *Snapshot) Context(ctx context.Context, recordID string) (ContextView, bool, error) {
	target, exists := s.recordIndex[recordID]
	if !exists {
		return ContextView{}, false, nil
	}
	if err := ctx.Err(); err != nil {
		return ContextView{}, false, err
	}
	count := 0
	for _, index := range s.eventRecords {
		if index > target {
			break
		}
		count++
	}
	beforeCount := count
	if count > 0 && s.eventRecords[count-1] == target {
		beforeCount--
	}
	before := conversation.ReplaySession(s.events[:beforeCount]).ContextMessages()
	after := conversation.ReplaySession(s.events[:count]).ContextMessages()
	through := 0
	if count > 0 {
		through = s.Records[s.eventRecords[count-1]].Line
	}
	return ContextView{
		Revision: s.Detail.Revision, AtRecord: recordID, ReplayedThrough: through,
		Complete: s.Records[target].Applied, Before: s.contextNodes(before, beforeCount), After: s.contextNodes(after, count),
	}, true, nil
}

func (s *Snapshot) contextNodes(messages []conversation.Message, count int) []ContextMessage {
	origins := map[string][]SourceRef{}
	metadataSources := []SourceRef{}
	compactionSource := SourceRef{}
	for i, event := range s.events[:count] {
		source := ref(s.Records[s.eventRecords[i]])
		switch e := event.(type) {
		case conversation.MessageAppended:
			id := e.Message.ID.String()
			origins[id] = append(origins[id], source)
			if isMetadata(e.Message) {
				metadataSources = append(metadataSources, source)
			}
		case conversation.SessionMetadataRefreshed:
			if isMetadata(e.Message) {
				metadataSources = append(metadataSources, source)
			}
		case conversation.SessionCompacted:
			origins[e.CompactionID.String()] = []SourceRef{source}
			compactionSource = source
		case conversation.SessionModelSet, conversation.SessionCwdSet, conversation.SessionReasoningEffortSet:
			if compactionSource.Line > 0 {
				metadataSources = append(metadataSources, source)
			}
		}
	}
	nodes := make([]ContextMessage, 0, len(messages))
	for _, message := range messages {
		kind, _ := message.Metadata["compactionKind"].(string)
		if kind == "" {
			kind = "persisted"
		}
		sources := append([]SourceRef{}, origins[message.ID.String()]...)
		if kind == "metadata" {
			sources = append([]SourceRef{}, metadataSources...)
			if compactionSource.Line > 0 {
				sources = append(sources, compactionSource)
			}
		}
		nodes = append(nodes, ContextMessage{Message: message, Kind: kind, Sources: sources})
	}
	return nodes
}

func isMetadata(message conversation.Message) bool {
	if !message.IsHidden() || len(message.Content) != 1 {
		return false
	}
	text, ok := message.Content[0].(conversation.TextBlock)
	if !ok {
		return false
	}
	var update conversation.MetadataUpdate
	return xml.Unmarshal([]byte(text.Text), &update) == nil
}
