package conversation

// ContextMessages projects live and replayed history for model requests without
// changing the persisted order. Metadata changes can arrive while tools execute;
// keep those messages after the complete batch of matching tool results.
func (s *Session) ContextMessages() []Message {
	messages := make([]Message, len(s.context))
	copy(messages, s.context)
	for start, message := range messages {
		if message.Role != RoleAssistant {
			continue
		}
		pending := make(map[string]struct{})
		for _, block := range message.Content {
			switch call := block.(type) {
			case ToolUseBlock:
				pending[call.ID] = struct{}{}
			case ShellCallBlock:
				pending[call.CallID] = struct{}{}
			case ApplyPatchCallBlock:
				pending[call.CallID] = struct{}{}
			}
		}
		if len(pending) == 0 {
			continue
		}

		results := make([]Message, 0)
		metadata := make([]Message, 0)
		for end := start + 1; end < len(messages); end++ {
			next := messages[end]
			if next.RoundID != message.RoundID {
				break
			}
			if next.IsHidden() && next.Metadata["kind"] == "metadata" {
				metadata = append(metadata, next)
				continue
			}
			if next.Role != RoleUser || len(next.Content) == 0 {
				break
			}

			matching := true
			for _, block := range next.Content {
				result, ok := block.(ToolResultBlock)
				if !ok {
					matching = false
					break
				}
				if _, ok := pending[result.ToolUseID]; !ok {
					matching = false
					break
				}
				delete(pending, result.ToolUseID)
			}
			if !matching {
				break
			}
			results = append(results, next)
			if len(pending) == 0 {
				// Only move metadata across a complete batch. Partial execution
				// and unrelated conversation boundaries retain their original order.
				if len(metadata) > 0 {
					copy(messages[start+1:], results)
					copy(messages[start+1+len(results):], metadata)
				}
				break
			}
		}
	}
	return messages
}
