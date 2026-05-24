/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/cli/chatstate"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/providers"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type chatOptions struct {
	agentRef   string
	modelRef   string
	sessionRef string
	newSession bool
	cwd        string
	thinking   string
	stream     bool
}

type chatResult struct {
	SessionID        uuid.UUID               `json:"sessionId"`
	AgentID          uuid.UUID               `json:"agentId"`
	ModelID          uuid.UUID               `json:"modelId"`
	Content          string                  `json:"content"`
	ReasoningContent string                  `json:"reasoningContent,omitempty"`
	TokenConsumed    int64                   `json:"tokenConsumed"`
	Messages         []models.ChatMessageDto `json:"messages"`
}

func newChatCmd() *cobra.Command {
	var opts chatOptions

	cmd := &cobra.Command{
		Use:   "chat [prompt]",
		Short: "Run a one-shot chat session from the CLI",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChatCommand(cmd, args, opts)
		},
	}

	cmd.Flags().StringVar(&opts.agentRef, "agent", "", "Agent name or ID")
	cmd.Flags().StringVar(&opts.modelRef, "model", "", "Chat model reference (provider/name, code, or ID)")
	cmd.Flags().StringVar(&opts.sessionRef, "session", "", "Reuse an existing session ID")
	cmd.Flags().BoolVar(&opts.newSession, "new", false, "Force a new session")
	cmd.Flags().StringVar(&opts.cwd, "cwd", "", "Explicit working directory to attach to the session")
	cmd.Flags().StringVar(&opts.thinking, "thinking", "off", "Thinking mode: off, on, or a supported thinking level")
	if thinkingFlag := cmd.Flags().Lookup("thinking"); thinkingFlag != nil {
		thinkingFlag.NoOptDefVal = "on"
	}
	cmd.Flags().BoolVar(&opts.stream, "stream", true, "Stream assistant output")

	return cmd
}

func runChatCommand(cmd *cobra.Command, args []string, opts chatOptions) error {
	prompt, err := readChatPrompt(cmd, args)
	if err != nil {
		return withExitCode(err, 2)
	}
	if strings.TrimSpace(prompt) == "" {
		return withExitCode(fmt.Errorf("prompt is required"), 2)
	}
	if opts.newSession && strings.TrimSpace(opts.sessionRef) != "" {
		return withExitCode(fmt.Errorf("--session and --new cannot be used together"), 2)
	}
	thinkingEnabled, thinkingLevel, err := parseChatThinkingFlag(opts.thinking)
	if err != nil {
		return withExitCode(err, 2)
	}
	if outputJSON && opts.stream {
		return withExitCode(fmt.Errorf("stream mode does not support --json; use --stream false with --json"), 2)
	}

	return runWithRuntime(cmd, false, false, func(runtime *Runtime) error {
		agentID, session, err := resolveChatSessionAndAgent(runtime.Backend, opts)
		if err != nil {
			return err
		}

		if session == nil {
			created, err := runtime.Backend.CreateSession(agentID)
			if err != nil {
				return withExitCode(fmt.Errorf("failed to create session: %w", err), 5)
			}
			session = created
		}

		if err := maybeSetChatSessionCwd(runtime.Backend, runtime.Local, session.ID, opts.cwd); err != nil {
			return err
		}

		modelID, _, err := resolveChatModel(runtime.Backend, agentID, session, opts.modelRef)
		if err != nil {
			return err
		}

		dto := &models.ChatDto{
			ModelID:       modelID,
			Message:       prompt,
			Thinking:      thinkingEnabled,
			ThinkingLevel: thinkingLevel,
		}

		if opts.stream {
			if err := runStreamingChat(cmd, runtime.Backend, session.ID, prompt, dto); err != nil {
				return withExitCode(err, 5)
			}
			return nil
		}

		messages, err := runtime.Backend.Chat(session.ID, dto)
		if err != nil {
			return withExitCode(err, 5)
		}

		if !outputJSON {
			result := buildChatResultFromMessages(messages)
			if result.Content != "" {
				return writeLine(cmd, "%s", result.Content)
			}
			return nil
		}

		completedSession, err := loadCompletedSession(runtime.Backend, session.ID)
		if err != nil {
			return err
		}
		result := buildChatResultFromSession(completedSession, modelID)
		return writeJSON(cmd, result)
	})
}

func parseChatThinkingFlag(raw string) (bool, string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "off", "false":
		return false, "", nil
	case "on", "true":
		return true, "", nil
	default:
		return true, strings.TrimSpace(raw), nil
	}
}

func readChatPrompt(cmd *cobra.Command, args []string) (string, error) {
	if len(args) == 0 {
		if stdinIsTerminal() {
			return "", nil
		}
		return readAllFromReader(cmd.InOrStdin())
	}

	if len(args) == 1 && args[0] == "-" {
		return readAllFromReader(cmd.InOrStdin())
	}

	return strings.TrimSpace(strings.Join(args, " ")), nil
}

func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func readAllFromReader(reader io.Reader) (string, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read stdin: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func resolveChatSessionAndAgent(b backend.Backend, opts chatOptions) (uuid.UUID, *models.ChatSessionDto, error) {
	if strings.TrimSpace(opts.sessionRef) != "" {
		sessionID, err := uuid.Parse(strings.TrimSpace(opts.sessionRef))
		if err != nil {
			return uuid.Nil, nil, withExitCode(fmt.Errorf("invalid session ID: %s", opts.sessionRef), 2)
		}
		session, err := b.GetSession(sessionID)
		if err != nil {
			return uuid.Nil, nil, err
		}
		if opts.agentRef != "" {
			agent, err := resolveAgentReference(b, opts.agentRef)
			if err != nil {
				return uuid.Nil, nil, err
			}
			if agent.ID != session.AgentID {
				return uuid.Nil, nil, withExitCode(fmt.Errorf("session %s belongs to agent %s, not %s", session.ID, session.AgentID, agent.ID), 2)
			}
		}
		return session.AgentID, session, nil
	}

	if opts.agentRef != "" {
		agent, err := resolveAgentReference(b, opts.agentRef)
		if err != nil {
			return uuid.Nil, nil, err
		}
		return agent.ID, nil, nil
	}

	agents, err := listAgentsAll(b)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if len(agents) == 0 {
		return uuid.Nil, nil, withExitCode(fmt.Errorf("no agents available; run agenty init first"), 2)
	}
	if agent := findDefaultAgent(agents); agent != nil {
		return agent.ID, nil, nil
	}
	return agents[0].ID, nil, nil
}

func maybeSetChatSessionCwd(b backend.Backend, local bool, sessionID uuid.UUID, cwdFlag string) error {
	cwdFlag = strings.TrimSpace(cwdFlag)
	if cwdFlag == "" {
		return nil
	}

	resolved := cwdFlag
	if local {
		absPath, err := filepath.Abs(cwdFlag)
		if err != nil {
			return withExitCode(fmt.Errorf("failed to resolve cwd: %w", err), 2)
		}
		resolved = absPath
	}

	var agentsMD *string
	if local {
		for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
			candidate := filepath.Join(resolved, name)
			data, err := os.ReadFile(candidate)
			if err == nil {
				content := string(data)
				agentsMD = &content
				break
			}
		}
	}

	if err := b.SetSessionCwd(sessionID, &resolved, agentsMD); err != nil {
		return withExitCode(fmt.Errorf("failed to set session cwd: %w", err), 5)
	}
	return nil
}

func resolveChatModel(b backend.Backend, agentID uuid.UUID, session *models.ChatSessionDto, modelRef string) (uuid.UUID, string, error) {
	if strings.TrimSpace(modelRef) != "" {
		model, err := resolveModelReference(b, modelRef, false)
		if err != nil {
			return uuid.Nil, "", err
		}
		if !modelSwitchable(*model) {
			return uuid.Nil, "", withExitCode(fmt.Errorf("model %q is not a configured chat model", modelDisplayName(*model)), 2)
		}
		return model.ID, modelDisplayName(*model), nil
	}
	return chatstate.ResolveInitialChatModel(b, agentID, session, session != nil)
}

func loadCompletedSession(b backend.Backend, sessionID uuid.UUID) (*models.ChatSessionDto, error) {
	session, err := b.GetSession(sessionID)
	if err != nil {
		return nil, withExitCode(fmt.Errorf("failed to load completed session: %w", err), 5)
	}
	return session, nil
}

func runStreamingChat(cmd *cobra.Command, b backend.Backend, sessionID uuid.UUID, prompt string, dto *models.ChatDto) error {
	renderer := newChatStreamRenderer(cmd.OutOrStdout())
	if err := renderer.WriteUser(prompt); err != nil {
		return err
	}
	err := b.StreamChat(cmd.Context(), sessionID, dto, func(event providers.StreamEvent) error {
		switch event.Type {
		case providers.EventReasoningDelta:
			if event.Reasoning != "" {
				return renderer.WriteReasoning(event.Reasoning)
			}
		case providers.EventContentDelta:
			if event.Content != "" {
				return renderer.WriteReply(event.Content)
			}
		case providers.EventToolCallDone:
			if event.ToolCall != nil {
				return renderer.WriteToolCall(*event.ToolCall)
			}
		case providers.EventMessageDone:
			if event.Message != nil {
				for _, toolCall := range event.Message.ToolCalls {
					if err := renderer.WriteToolCall(toolCall); err != nil {
						return err
					}
				}
			}
			return renderer.FinishAssistantMessage()
		case providers.EventToolResult:
			if event.ToolResult != nil {
				return renderer.WriteToolResult(*event.ToolResult)
			}
		case providers.EventError:
			return errors.New(event.Error)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return renderer.Finish()
}

func buildChatResultFromSession(session *models.ChatSessionDto, modelID uuid.UUID) chatResult {
	result := chatResult{
		ModelID: modelID,
	}
	if session == nil {
		return result
	}
	result.SessionID = session.ID
	result.AgentID = session.AgentID
	result.TokenConsumed = session.TokenConsumed
	result.Messages = append(result.Messages, session.Messages...)
	for i := len(session.Messages) - 1; i >= 0; i-- {
		msg := session.Messages[i]
		if msg.Role != models.RoleAssistant {
			continue
		}
		result.Content = msg.Content
		if msg.ReasoningContent != "" {
			result.ReasoningContent = msg.ReasoningContent
		}
		break
	}
	return result
}

func buildChatResultFromMessages(messages *[]*models.ChatMessageDto) chatResult {
	result := chatResult{}
	if messages == nil {
		return result
	}
	for _, msg := range *messages {
		if msg == nil || msg.Role != models.RoleAssistant {
			continue
		}
		result.Content = msg.Content
		if msg.ReasoningContent != "" {
			result.ReasoningContent = msg.ReasoningContent
		}
	}
	return result
}

type chatStreamSection int

const (
	chatStreamSectionNone chatStreamSection = iota
	chatStreamSectionReasoning
	chatStreamSectionReply
)

type chatStreamRenderer struct {
	out              io.Writer
	section          chatStreamSection
	atLineStart      bool
	printedToolCalls map[string]bool
}

func newChatStreamRenderer(out io.Writer) *chatStreamRenderer {
	return &chatStreamRenderer{
		out:              out,
		printedToolCalls: make(map[string]bool),
	}
}

func (r *chatStreamRenderer) WriteUser(prompt string) error {
	return r.writeBlock("user", prompt)
}

func (r *chatStreamRenderer) WriteReasoning(text string) error {
	if err := r.beginTextSection(chatStreamSectionReasoning, "assistant thinking"); err != nil {
		return err
	}
	return r.writeDelta(text)
}

func (r *chatStreamRenderer) WriteReply(text string) error {
	if err := r.beginTextSection(chatStreamSectionReply, "assistant reply"); err != nil {
		return err
	}
	return r.writeDelta(text)
}

func (r *chatStreamRenderer) WriteToolCall(toolCall models.ToolCall) error {
	if toolCall.ID != "" && r.printedToolCalls[toolCall.ID] {
		return nil
	}
	if err := r.endTextSection(); err != nil {
		return err
	}
	if toolCall.ID != "" {
		r.printedToolCalls[toolCall.ID] = true
	}
	if _, err := fmt.Fprintln(r.out, "assistant tool call:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(r.out, "  name: %s\n", toolCall.Name); err != nil {
		return err
	}
	if toolCall.ID != "" {
		if _, err := fmt.Fprintf(r.out, "  id: %s\n", toolCall.ID); err != nil {
			return err
		}
	}
	if toolCall.Arguments != "" {
		if _, err := fmt.Fprintln(r.out, "  arguments:"); err != nil {
			return err
		}
		if err := writeIndentedText(r.out, toolCall.Arguments, "    "); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(r.out)
	return err
}

func (r *chatStreamRenderer) WriteToolResult(toolResult models.ToolResult) error {
	if err := r.endTextSection(); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(r.out, "tool result:"); err != nil {
		return err
	}
	name := toolResult.Name
	if name == "" {
		name = "tool"
	}
	if _, err := fmt.Fprintf(r.out, "  name: %s\n", name); err != nil {
		return err
	}
	if toolResult.CallID != "" {
		if _, err := fmt.Fprintf(r.out, "  id: %s\n", toolResult.CallID); err != nil {
			return err
		}
	}
	status := "ok"
	if toolResult.IsError {
		status = "error"
	}
	if _, err := fmt.Fprintf(r.out, "  status: %s\n", status); err != nil {
		return err
	}
	if toolResult.Content != "" {
		if _, err := fmt.Fprintln(r.out, "  content:"); err != nil {
			return err
		}
		if err := writeIndentedText(r.out, toolResult.Content, "    "); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(r.out)
	return err
}

func (r *chatStreamRenderer) FinishAssistantMessage() error {
	return r.endTextSection()
}

func (r *chatStreamRenderer) Finish() error {
	return r.endTextSection()
}

func (r *chatStreamRenderer) writeBlock(label, content string) error {
	if err := r.endTextSection(); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(r.out, "%s:\n", label); err != nil {
		return err
	}
	if err := writeIndentedText(r.out, content, "  "); err != nil {
		return err
	}
	_, err := fmt.Fprintln(r.out)
	return err
}

func (r *chatStreamRenderer) beginTextSection(section chatStreamSection, label string) error {
	if r.section == section {
		return nil
	}
	if err := r.endTextSection(); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(r.out, "%s:\n", label); err != nil {
		return err
	}
	r.section = section
	r.atLineStart = true
	return nil
}

func (r *chatStreamRenderer) writeDelta(text string) error {
	for len(text) > 0 {
		if r.atLineStart {
			if _, err := fmt.Fprint(r.out, "  "); err != nil {
				return err
			}
			r.atLineStart = false
		}
		idx := strings.IndexByte(text, '\n')
		if idx == -1 {
			if _, err := fmt.Fprint(r.out, text); err != nil {
				return err
			}
			return nil
		}
		if _, err := fmt.Fprint(r.out, text[:idx+1]); err != nil {
			return err
		}
		text = text[idx+1:]
		r.atLineStart = true
	}
	return nil
}

func (r *chatStreamRenderer) endTextSection() error {
	if r.section == chatStreamSectionNone {
		return nil
	}
	if !r.atLineStart {
		if _, err := fmt.Fprintln(r.out); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(r.out); err != nil {
		return err
	}
	r.section = chatStreamSectionNone
	r.atLineStart = false
	return nil
}

func writeIndentedText(out io.Writer, text, indent string) error {
	if text == "" {
		_, err := fmt.Fprintln(out, indent)
		return err
	}
	for len(text) > 0 {
		idx := strings.IndexByte(text, '\n')
		if idx == -1 {
			_, err := fmt.Fprintf(out, "%s%s\n", indent, text)
			return err
		}
		if _, err := fmt.Fprintf(out, "%s%s\n", indent, text[:idx]); err != nil {
			return err
		}
		text = text[idx+1:]
	}
	return nil
}
