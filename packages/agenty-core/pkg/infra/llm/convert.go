package llm

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/catalog"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func validateRequest(request modelRequest) error {
	if request.MaxOutputTokens <= 0 {
		return invalidRequest("max output tokens must be greater than zero")
	}
	if request.ReasoningBudgetTokens < 0 {
		return invalidRequest("reasoning budget tokens must not be negative")
	}
	if request.ReasoningEffort != "" && !request.ReasoningEffort.Valid() {
		return invalidRequest("unknown reasoning effort %q", request.ReasoningEffort)
	}
	for index, message := range request.Messages {
		if !message.Role.Valid() {
			return invalidRequest("message %d has unknown role %q", index, message.Role)
		}
	}

	return nil
}

func nativeReasoningEffort(model catalog.Model, effort shared.ReasoningEffort) (string, error) {
	if effort == "" || effort == shared.ReasoningOff {
		return "", nil
	}
	if native, ok := model.ReasoningEffortMapping[string(effort)]; ok && native == effort {
		return string(effort), nil
	}

	matches := make([]string, 0, 1)
	for native, mapped := range model.ReasoningEffortMapping {
		if mapped == effort {
			matches = append(matches, native)
		}
	}
	sort.Strings(matches)

	switch len(matches) {
	case 0:
		return "", invalidRequest("model %q does not support reasoning effort %q", model.Slug, effort)
	case 1:
		return matches[0], nil
	default:
		return "", invalidRequest("model %q maps reasoning effort %q ambiguously to %s", model.Slug, effort, strings.Join(matches, ", "))
	}
}

func systemPrompt(request modelRequest) (string, error) {
	parts := make([]string, 0, 2)
	if prompt := strings.TrimSpace(request.SystemPrompt); prompt != "" {
		parts = append(parts, prompt)
	}
	for index, message := range request.Messages {
		if message.Role != conversation.RoleSystem {
			continue
		}
		text, err := textContent(message.Content)
		if err != nil {
			return "", fmt.Errorf("llm: convert system message %d: %w", index, err)
		}
		if text != "" {
			parts = append(parts, text)
		}
	}

	return strings.Join(parts, "\n\n"), nil
}

func textContent(content conversation.Content) (string, error) {
	var builder strings.Builder
	for _, block := range content {
		switch value := block.(type) {
		case conversation.TextBlock:
			builder.WriteString(value.Text)
		case conversation.ShellCallOutputBlock:
			encoded, err := marshalShellCallOutput(value)
			if err != nil {
				return "", fmt.Errorf("encode shell output: %w", err)
			}
			builder.WriteString(encoded)
		default:
			return "", unsupportedContent("expected text block, got %q", block.BlockType())
		}
	}

	return builder.String(), nil
}

type shellCallOutputWire struct {
	Type            string                   `json:"type"`
	CallID          string                   `json:"call_id"`
	MaxOutputLength int64                    `json:"max_output_length"`
	Output          []shellCommandOutputWire `json:"output"`
}

type shellCommandOutputWire struct {
	Stdout  string           `json:"stdout"`
	Stderr  string           `json:"stderr"`
	Outcome shellOutcomeWire `json:"outcome"`
}

type shellOutcomeWire struct {
	Type     string `json:"type"`
	ExitCode *int64 `json:"exit_code,omitempty"`
}

func marshalShellCallOutput(value conversation.ShellCallOutputBlock) (string, error) {
	output := make([]shellCommandOutputWire, 0, len(value.Output))
	for _, command := range value.Output {
		output = append(output, shellCommandOutputWire{
			Stdout: command.Stdout,
			Stderr: command.Stderr,
			Outcome: shellOutcomeWire{
				Type: command.Outcome.Type, ExitCode: command.Outcome.ExitCode,
			},
		})
	}

	encoded, err := json.MarshalString(shellCallOutputWire{
		Type:            string(conversation.BlockShellOutput),
		CallID:          value.CallID,
		MaxOutputLength: value.MaxOutputLength,
		Output:          output,
	})
	if err != nil {
		return "", err
	}

	return encoded, nil
}

func shellCallOutputObject(value conversation.ShellCallOutputBlock) (map[string]any, error) {
	encoded, err := marshalShellCallOutput(value)
	if err != nil {
		return nil, err
	}
	return rawObject(shared.RawJSON(encoded), "shell output")
}

func rawObject(raw shared.RawJSON, field string) (map[string]any, error) {
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, invalidRequest("%s is not a JSON object: %v", field, err)
	}
	if value == nil {
		return nil, invalidRequest("%s must be a JSON object", field)
	}

	return value, nil
}

func imageURL(block conversation.ImageBlock) (string, error) {
	if block.Data != "" && block.URI != "" {
		return "", invalidRequest("image must contain either data or URI, not both")
	}
	if block.URI != "" {
		return block.URI, nil
	}
	if block.Data == "" || block.MimeType == "" {
		return "", invalidRequest("inline image requires MIME type and base64 data")
	}
	if _, err := base64.StdEncoding.DecodeString(block.Data); err != nil {
		return "", invalidRequest("inline image data is not valid base64: %v", err)
	}

	return fmt.Sprintf("data:%s;base64,%s", block.MimeType, block.Data), nil
}

func emit(handler modelStreamHandler, event modelStreamEvent) error {
	if handler == nil {
		return nil
	}
	if err := handler(event); err != nil {
		return fmt.Errorf("llm: handle stream event %q: %w", event.Type, err)
	}

	return nil
}

func rawJSON(value any) (shared.RawJSON, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("llm: marshal JSON value: %w", err)
	}

	return shared.RawJSON(data), nil
}
