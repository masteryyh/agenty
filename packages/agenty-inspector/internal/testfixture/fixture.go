// Package testfixture supplies synthetic sessions without accessing user data.
package testfixture

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func Events() []shared.Event {
	cwd := "/workspace/example"
	model := shared.ModelRef{ProviderCode: "example", ModelCode: "reasoner"}
	session := conversation.StartSession("developer", model, 128000, shared.ReasoningEffort("high"), &cwd)
	session.SetTitle("Trace a provider configuration change")
	round, err := session.StartRound()
	must(err)
	_, err = session.AppendHiddenUserMessage(round, conversation.Text("<metadata><cwd>/workspace/example</cwd><model>reasoner</model><provider>example</provider><timezone>UTC</timezone></metadata>"))
	must(err)
	_, err = session.AppendUserMessage(round, conversation.Text("Inspect the provider configuration and explain how the request is built."))
	must(err)
	_, err = session.AppendAssistantMessage(round, conversation.Content{
		conversation.ReasoningBlock{Reasoning: "I will inspect the configuration and trace its callers.", Signature: "synthetic-signature"},
		conversation.TextBlock{Text: "The configuration is loaded at startup. Let me check the defaults."},
		conversation.ToolUseBlock{ID: "read-1", Name: "read_file", Input: shared.RawJSON(`{"path":"config.go"}`)},
		conversation.ShellCallBlock{CallID: "shell-1", Commands: []string{"go test ./pkg/config"}, TimeoutMs: 10000},
		conversation.ApplyPatchCallBlock{CallID: "patch-1", Source: conversation.ApplyPatchSourceCustom, Patch: "*** Begin Patch\n*** Update File: config.go\n@@\n-old\n+new\n*** End Patch"},
	}, model, &conversation.TokenUsage{Input: 340, Output: 120, Total: 460})
	must(err)
	exitCode := int64(0)
	_, err = session.AppendUserMessage(round, conversation.Content{
		conversation.ToolResultBlock{ToolUseID: "read-1", Content: conversation.Content{
			conversation.TextBlock{Text: "func Load() Config { return Config{Timeout: 30} }"},
			conversation.ImageBlock{MimeType: "image/png", URI: "https://example.invalid/diagram.png"},
		}},
		conversation.ShellCallOutputBlock{CallID: "shell-1", Output: []conversation.ShellCommandOutput{
			{Stdout: "ok example/pkg/config 0.014s", Outcome: conversation.ShellOutcome{Type: "exit", ExitCode: &exitCode}},
		}},
		conversation.ToolResultBlock{ToolUseID: "patch-1", Content: conversation.Text("Applied patch successfully.")},
	})
	must(err)
	_, err = session.AppendAssistantMessage(round, conversation.Text("## Findings\n\nThe provider configuration uses a **30 second timeout**.\n\n- Read configuration\n- Verified defaults\n- Updated the requested field\n\n```go\nTimeout: 30\n```"), model, &conversation.TokenUsage{Input: 520, Output: 180, Total: 700})
	must(err)
	must(session.CompleteRound(round, conversation.RoundCompleted, conversation.TokenUsage{Input: 860, Output: 300, Total: 1160}, nil))
	_, err = session.Compact(conversation.CompactionInput{CompactionID: uuid.New(), Trigger: conversation.CompactionTriggerManual,
		Summary: "The provider configuration has a 30 second timeout. All targeted tests passed. Next: verify cancellation behavior.", ContextTokensBefore: 1500,
		Usage: conversation.TokenUsage{Input: 1100, Output: 90, Total: 1190}})
	must(err)
	session.SetModel(shared.ModelRef{ProviderCode: "example", ModelCode: "reasoner-small"}, 32000)
	round, err = session.StartRound()
	must(err)
	_, err = session.AppendUserMessage(round, conversation.Text("Now check cancellation behavior."))
	must(err)
	return session.PendingEvents()
}

func Encode(events []shared.Event) []byte {
	raw := []byte{}
	for i, event := range events {
		line, err := shared.EncodeEvent(int64(i+1), event)
		must(err)
		raw = append(raw, line...)
		raw = append(raw, '\n')
	}
	return raw
}

func must(err error) {
	if err != nil {
		panic(fmt.Sprintf("invalid test fixture: %v", err))
	}
}
