package inspection

import (
	"fmt"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

func (s *Snapshot) validate() {
	rounds := map[uuid.UUID]conversation.RoundStatus{}
	messages := map[uuid.UUID]bool{}
	tools := map[string]*ToolRelation{}
	order := []string{}
	var sessionID uuid.UUID
	for i, event := range s.events {
		record := s.Records[s.eventRecords[i]]
		var identity struct {
			SessionID uuid.UUID `json:"sessionId"`
		}
		detail, _ := s.Record(record.ID)
		if detail.Envelope != nil {
			if err := json.Unmarshal(detail.Envelope.Payload, &identity); err != nil {
				s.issue("invalid_session_id", "error", err.Error(), record)
			}
		}
		if sessionID != uuid.Nil && identity.SessionID != sessionID {
			s.issue("session_id_mismatch", "error", "Event belongs to a different session.", record)
		}
		switch e := event.(type) {
		case conversation.SessionStarted:
			if sessionID != uuid.Nil {
				s.issue("duplicate_start", "error", "Session has more than one start event.", record)
			}
			sessionID = e.SessionID
			if s.Detail.Entry.SessionID != "" && s.Detail.Entry.SessionID != e.SessionID.String() {
				s.issue("filename_id_mismatch", "warning", "Session ID differs from the transcript filename.", record)
			}
		case conversation.RoundStarted:
			if _, exists := rounds[e.RoundID]; exists {
				s.issue("duplicate_round", "error", "Round ID is repeated.", record)
			}
			rounds[e.RoundID] = conversation.RoundRunning
		case conversation.RoundEnded:
			status, exists := rounds[e.RoundID]
			if !exists {
				s.issue("orphan_round_end", "error", "Round end has no preceding start.", record)
			} else if status.Terminal() {
				s.issue("duplicate_round_end", "error", "Round has multiple terminal events.", record)
			}
			if !e.Status.Terminal() {
				s.issue("nonterminal_round_end", "error", "Round end contains a non-terminal status.", record)
			}
			rounds[e.RoundID] = e.Status
		case conversation.MessageAppended:
			status, exists := rounds[e.Message.RoundID]
			if !exists {
				s.issue("orphan_message", "error", "Message references a round that has not started.", record)
			} else if status.Terminal() {
				s.issue("message_after_end", "error", "Message was appended after the round ended.", record)
			}
			if messages[e.Message.ID] {
				s.issue("duplicate_message", "error", "Message ID is repeated.", record)
			}
			messages[e.Message.ID] = true
			walkTools(e.Message.Content, "content", "", func(id, name, path string, result, failed bool) {
				key := e.Message.RoundID.String() + "\x00" + id
				relation := tools[key]
				if relation == nil {
					relation = &ToolRelation{ID: id, Name: name, RoundID: e.Message.RoundID.String(),
						Calls: []SourceRef{}, Results: []SourceRef{}, Status: "matched"}
					tools[key] = relation
					order = append(order, key)
				}
				source := ref(record)
				source.ContentPath = path
				if result {
					relation.Results = append(relation.Results, source)
				} else {
					relation.Name = name
					relation.Calls = append(relation.Calls, source)
				}
				if failed {
					relation.Status = "error"
				}
			})
		case conversation.SessionCompacted:
			if strings.TrimSpace(e.Summary) == "" {
				s.issue("empty_summary", "error", "Compaction contains an empty summary.", record)
			}
		}
		if sessionID == uuid.Nil {
			s.issue("missing_start", "error", "No session start precedes this event.", record)
		}
	}
	if len(s.events) == 0 {
		s.Detail.Diagnostics = append(s.Detail.Diagnostics, Diagnostic{
			Code: "no_replayable_events", Severity: "warning", Message: "No replayable events were found.",
		})
	}
	for _, round := range s.Detail.Rounds {
		if !round.Status.Terminal() {
			s.Detail.Diagnostics = append(s.Detail.Diagnostics, Diagnostic{
				Code: "no_terminal_event", Severity: "info", Message: "No terminal event recorded; execution may have stopped or still be active.",
				Source: round.Source,
			})
		}
	}
	for _, key := range order {
		relation := tools[key]
		switch {
		case len(relation.Calls) > 1 || len(relation.Results) > 1:
			relation.Status = "ambiguous"
		case len(relation.Calls) == 0:
			relation.Status = "missing_call"
		case len(relation.Results) == 0:
			relation.Status = "missing_result"
		case relation.Results[0].Line < relation.Calls[0].Line:
			relation.Status = "result_before_call"
		}
		if relation.Status != "matched" && relation.Status != "error" {
			sources := append(append([]SourceRef{}, relation.Calls...), relation.Results...)
			s.Detail.Diagnostics = append(s.Detail.Diagnostics, Diagnostic{
				Code: relation.Status, Severity: "warning", Message: diagnosticMessage(*relation),
				Source: sources[0],
			})
		}
		s.Detail.Tools = append(s.Detail.Tools, *relation)
	}
}

func diagnosticMessage(relation ToolRelation) string {
	if relation.Status == "ambiguous" {
		name := relation.Name
		if name == "" {
			name = "unknown tool"
		}
		message := fmt.Sprintf(
			"Tool %q (%s) has %d calls and %d results with the same call ID in this round. A call ID should identify one call and one result, so Inspector cannot determine a unique pairing.",
			relation.ID,
			name,
			len(relation.Calls),
			len(relation.Results),
		)
		if relation.Name == "shell" {
			message += " Multiple commands inside one shell result are expected and are not counted as separate results."
		}
		return message
	}
	return fmt.Sprintf("Tool %s: %s.", relation.ID, strings.ReplaceAll(relation.Status, "_", " "))
}

func walkTools(content conversation.Content, path, resultOwnerID string, visit func(string, string, string, bool, bool)) {
	for index, block := range content {
		blockPath := fmt.Sprintf("%s/%d", path, index)
		switch b := block.(type) {
		case conversation.ToolUseBlock:
			visit(b.ID, b.Name, blockPath, false, false)
		case conversation.ToolResultBlock:
			// ToolResultBlock 是结果归属节点；内部同 ID 的 shell 输出只是 payload，不能重复计数。
			visit(b.ToolUseID, "", blockPath, true, b.IsError || shellOutputFailed(b.Content))
			walkTools(b.Content, blockPath+"/content", b.ToolUseID, visit)
		case conversation.ShellCallBlock:
			visit(b.CallID, "shell", blockPath, false, false)
		case conversation.ShellCallOutputBlock:
			if resultOwnerID != "" && b.CallID == resultOwnerID {
				continue
			}
			failed := false
			for _, output := range b.Output {
				if output.Outcome.Type == "timeout" || (output.Outcome.ExitCode != nil && *output.Outcome.ExitCode != 0) {
					failed = true
				}
			}
			visit(b.CallID, "shell", blockPath, true, failed)
		case conversation.ApplyPatchCallBlock:
			visit(b.CallID, "apply_patch", blockPath, false, false)
		}
	}
}

func shellOutputFailed(content conversation.Content) bool {
	for _, block := range content {
		switch b := block.(type) {
		case conversation.ShellCallOutputBlock:
			for _, output := range b.Output {
				if output.Outcome.Type == "timeout" || (output.Outcome.ExitCode != nil && *output.Outcome.ExitCode != 0) {
					return true
				}
			}
		case conversation.ToolResultBlock:
			if shellOutputFailed(b.Content) {
				return true
			}
		}
	}
	return false
}
