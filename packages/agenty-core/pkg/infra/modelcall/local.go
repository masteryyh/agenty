package modelcall

import (
	"context"
	"fmt"
	"slices"
	"strings"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	domain "github.com/masteryyh/agenty-core/pkg/domain/localinference"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
	localruntime "github.com/masteryyh/agenty-core/pkg/infra/localinference"
	"github.com/masteryyh/agenty-core/pkg/infra/workeripc"
)

type LocalRuntime interface {
	WithModel(context.Context, *domain.Model, func(localruntime.Lease) error) error
}
type localCaller struct {
	model   ModelCallConfig
	runtime LocalRuntime
}

func (c *localCaller) Invoke(ctx context.Context, request ModelCallRequest) (*ModelCallResponse, error) {
	return c.Stream(ctx, request, nil)
}

type localMessage struct {
	Role             string          `json:"role"`
	Content          *string         `json:"content"`
	ReasoningContent string          `json:"reasoningContent,omitempty"`
	ToolCalls        []localToolCall `json:"toolCalls,omitempty"`
	ToolCallID       string          `json:"toolCallId,omitempty"`
}
type localFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}
type localToolCall struct {
	ID       string        `json:"id"`
	Function localFunction `json:"function"`
}
type localRenderRequest struct {
	SystemPrompt        string         `json:"systemPrompt"`
	Messages            []localMessage `json:"messages"`
	Tools               []any          `json:"tools"`
	AddGenerationPrompt bool           `json:"addGenerationPrompt"`
	TemplateName        string         `json:"templateName,omitempty"`
	TemplateKwargs      map[string]any `json:"templateKwargs"`
}

func localRequest(model ModelCallConfig, request ModelCallRequest) (localRenderRequest, error) {
	if err := model.Local.Validate(); err != nil {
		return localRenderRequest{}, invalidRequest("%v", err)
	}
	if request.OutputFormat != nil {
		return localRenderRequest{}, invalidRequest("local worker does not support schema-constrained generation")
	}
	if request.ReasoningBudgetTokens != 0 {
		return localRenderRequest{}, invalidRequest("local worker does not support a separate thinking token budget")
	}
	effort := request.ReasoningEffort
	thinking := effort.Enabled()
	if thinking && (model.Local.ThinkingEffort == "" || string(effort) != model.Local.ThinkingEffort) {
		return localRenderRequest{}, fmt.Errorf("%w: %s", ErrUnsupportedReasoningEffort, effort)
	}
	rendered := localRenderRequest{SystemPrompt: request.SystemPrompt, Messages: make([]localMessage, 0), Tools: make([]any, 0),
		AddGenerationPrompt: true, TemplateName: model.Local.TemplateName, TemplateKwargs: map[string]any{"enable_thinking": thinking}}
	for _, tool := range request.Tools {
		rendered.Tools = append(rendered.Tools, map[string]any{"type": "function", "function": map[string]any{
			"name": tool.Name, "description": tool.Description, "parameters": tool.InputSchema}})
	}
	for _, message := range request.Messages {
		text, reasoning := "", ""
		calls := make([]localToolCall, 0)
		results := make([]localMessage, 0)
		for _, block := range message.Content {
			switch value := block.(type) {
			case conversation.TextBlock:
				text += value.Text
			case conversation.ReasoningBlock:
				reasoning += value.Reasoning
			case conversation.ToolUseBlock:
				arguments, err := rawObject(value.Input, "local tool arguments")
				if err != nil {
					return rendered, err
				}
				calls = append(calls, localToolCall{ID: value.ID, Function: localFunction{Name: value.Name, Arguments: arguments}})
			case conversation.ToolResultBlock:
				result, err := textContent(value.Content)
				if err != nil {
					return rendered, err
				}
				results = append(results, localMessage{Role: "tool", Content: &result, ToolCallID: value.ToolUseID})
			default:
				return rendered, unsupportedContent("local inference does not support %s", block.BlockType())
			}
		}
		if len(results) > 0 && (text != "" || len(calls) > 0 || reasoning != "") {
			return rendered, invalidRequest("mixed tool result and conversation content")
		}
		if len(results) > 0 {
			rendered.Messages = append(rendered.Messages, results...)
			continue
		}
		rendered.Messages = append(rendered.Messages, localMessage{Role: string(message.Role), Content: &text, ReasoningContent: reasoning, ToolCalls: calls})
	}
	return rendered, nil
}

func (c *localCaller) Stream(ctx context.Context, request ModelCallRequest, handler ModelCallStreamHandler) (*ModelCallResponse, error) {
	input, err := localRequest(c.model, request)
	if err != nil {
		return nil, err
	}
	var response *ModelCallResponse
	err = c.runtime.WithModel(ctx, c.model.Local, func(lease localruntime.Lease) error {
		ctx = lease.Context
		ids, err := lease.Codec.ChatEncode(input)
		if err != nil {
			return err
		}
		window := lease.Model.ContextWindow
		if c.model.ContextWindow > 0 {
			window = min(window, c.model.ContextWindow)
		}
		if len(ids) > lease.Client.MaxInputTokens || request.MaxOutputTokens <= 0 || request.MaxOutputTokens > lease.Client.MaxOutputTokens {
			return fmt.Errorf("local request exceeds negotiated worker token limits")
		}
		if int64(len(ids))+request.MaxOutputTokens > int64(window) {
			return fmt.Errorf("local_context_exceeded: %d input + %d output exceeds %d", len(ids), request.MaxOutputTokens, window)
		}
		decoder, err := lease.Codec.Decoder()
		if err != nil {
			return err
		}
		defer decoder.Close()
		parser := newLocalParser(c.model.Local.OutputParser, request.Tools, handler)
		if c.model.Local.OutputParser == "qwen3_5" && request.ReasoningEffort.Enabled() {
			parser.mode = "reasoning"
		}
		var terminal struct {
			FinishReason string `json:"finishReason"`
			Usage        struct {
				Input  int64 `json:"inputTokens"`
				Output int64 `json:"outputTokens"`
			} `json:"usage"`
		}
		var generated int64
		params := map[string]any{"modelHandle": lease.Model.ModelHandle, "tokenIds": ids, "maxOutputTokens": request.MaxOutputTokens,
			"stopTokenIds": lease.Codec.Metadata.StopTokenIDs}
		err = lease.Client.Call(ctx, "generate", params, &terminal, func(frame workeripc.Frame) error {
			if frame.Event != "tokens" {
				return fmt.Errorf("unknown inference event %s", frame.Event)
			}
			var batch struct {
				IDs []uint32 `json:"tokenIds"`
			}
			if err := json.Unmarshal(frame.Data, &batch); err != nil {
				return err
			}
			if len(batch.IDs) == 0 {
				return fmt.Errorf("empty token event")
			}
			generated += int64(len(batch.IDs))
			if generated > request.MaxOutputTokens {
				return fmt.Errorf("worker exceeded output token budget")
			}
			visible := make([]uint32, 0, len(batch.IDs))
			for _, id := range batch.IDs {
				if !slices.Contains(lease.Codec.Metadata.StopTokenIDs, id) {
					visible = append(visible, id)
				}
			}
			text, err := decoder.Push(visible, false)
			if err != nil {
				return err
			}
			return parser.push(text, false)
		})
		if err != nil {
			return err
		}
		if terminal.FinishReason == "cancelled" {
			return context.Canceled
		}
		if terminal.FinishReason != "eos" && terminal.FinishReason != "maxTokens" {
			return fmt.Errorf("invalid worker finish reason %s", terminal.FinishReason)
		}
		if terminal.Usage.Input != int64(len(ids)) || terminal.Usage.Output != generated {
			return fmt.Errorf("worker usage does not match token stream")
		}
		tail, err := decoder.Push(nil, true)
		if err != nil {
			return err
		}
		if err := parser.push(tail, true); err != nil {
			return err
		}
		reason := ModelCallStopReasonEndTurn
		if terminal.FinishReason == "maxTokens" {
			reason = ModelCallStopReasonMaxTokens
		}
		if parser.calls > 0 {
			reason = ModelCallStopReasonToolUse
		}
		response = &ModelCallResponse{ID: uuid.NewString(), Model: c.model.ModelCode, Content: parser.content,
			StopReason: reason, Usage: conversation.TokenUsage{Input: terminal.Usage.Input, Output: generated, Total: terminal.Usage.Input + generated}}
		if handler != nil {
			return handler(ModelCallStreamEvent{Type: ModelCallStreamEventCompleted, Response: response})
		}
		return nil
	})
	return response, err
}

type localParser struct {
	format  string
	mode    string
	pending string
	content conversation.Content
	tools   map[string]ToolDefinition
	handler ModelCallStreamHandler
	calls   int
}

func newLocalParser(format string, tools []ToolDefinition, handler ModelCallStreamHandler) *localParser {
	p := &localParser{format: format, mode: "text", content: make(conversation.Content, 0), tools: make(map[string]ToolDefinition), handler: handler}
	for _, tool := range tools {
		p.tools[tool.Name] = tool
	}
	return p
}
func (p *localParser) emitText(text string) error {
	if text == "" {
		return nil
	}
	kind := ModelCallStreamEventTextDelta
	if p.mode == "reasoning" {
		kind = ModelCallStreamEventReasoningDelta
		if len(p.content) > 0 {
			if block, ok := p.content[len(p.content)-1].(conversation.ReasoningBlock); ok {
				block.Reasoning += text
				p.content[len(p.content)-1] = block
			} else {
				p.content = append(p.content, conversation.ReasoningBlock{Reasoning: text})
			}
		} else {
			p.content = append(p.content, conversation.ReasoningBlock{Reasoning: text})
		}
	} else {
		if len(p.content) > 0 {
			if block, ok := p.content[len(p.content)-1].(conversation.TextBlock); ok {
				block.Text += text
				p.content[len(p.content)-1] = block
			} else {
				p.content = append(p.content, conversation.TextBlock{Text: text})
			}
		} else {
			p.content = append(p.content, conversation.TextBlock{Text: text})
		}
	}
	if p.handler != nil {
		return p.handler(ModelCallStreamEvent{Type: kind, Delta: text, Index: len(p.content) - 1})
	}
	return nil
}
func (p *localParser) push(text string, finish bool) error {
	p.pending += text
	if len(p.pending) > 1<<20 {
		return fmt.Errorf("local output block exceeds 1 MiB")
	}
	for p.pending != "" {
		if p.mode == "tool" {
			end := strings.Index(p.pending, "</tool_call>")
			if end < 0 {
				if finish {
					return fmt.Errorf("incomplete local tool call")
				}
				return nil
			}
			if err := p.tool(p.pending[:end]); err != nil {
				return err
			}
			p.pending = p.pending[end+len("</tool_call>"):]
			p.mode = "text"
			continue
		}
		tags := []string{"<think>", "</think>", "<tool_call>"}
		position, tag := -1, ""
		for _, candidate := range tags {
			if at := strings.Index(p.pending, candidate); at >= 0 && (position < 0 || at < position) {
				position, tag = at, candidate
			}
		}
		if position >= 0 {
			if err := p.emitText(p.pending[:position]); err != nil {
				return err
			}
			p.pending = p.pending[position+len(tag):]
			switch tag {
			case "<think>":
				p.mode = "reasoning"
			case "</think>":
				p.mode = "text"
			case "<tool_call>":
				p.mode = "tool"
			}
			continue
		}
		keep := 0
		if !finish {
			for _, candidate := range tags {
				for n := 1; n < len(candidate) && n <= len(p.pending); n++ {
					if strings.HasSuffix(p.pending, candidate[:n]) {
						keep = max(keep, n)
					}
				}
			}
		}
		n := len(p.pending) - keep
		if err := p.emitText(p.pending[:n]); err != nil {
			return err
		}
		p.pending = p.pending[n:]
		return nil
	}
	if finish && p.mode == "tool" {
		return fmt.Errorf("incomplete local tool call")
	}
	return nil
}
func (p *localParser) tool(source string) error {
	var function localFunction
	if p.format == "qwen3" {
		if err := json.UnmarshalString(strings.TrimSpace(source), &function); err != nil {
			return fmt.Errorf("invalid Qwen tool JSON: %w", err)
		}
	} else {
		source = strings.TrimSpace(source)
		if !strings.HasPrefix(source, "<function=") {
			return fmt.Errorf("expected Qwen3.5 function block")
		}
		end := strings.IndexByte(source, '>')
		if end < 0 {
			return fmt.Errorf("incomplete function name")
		}
		function.Name = source[len("<function="):end]
		tool, ok := p.tools[function.Name]
		if !ok {
			return fmt.Errorf("model requested unknown tool %q", function.Name)
		}
		function.Arguments = make(map[string]any)
		source = strings.TrimSpace(source[end+1:])
		for !strings.HasPrefix(source, "</function>") {
			if !strings.HasPrefix(source, "<parameter=") {
				return fmt.Errorf("expected tool parameter")
			}
			nameEnd := strings.IndexByte(source, '>')
			if nameEnd < 0 {
				return fmt.Errorf("incomplete parameter name")
			}
			name := source[len("<parameter="):nameEnd]
			source = source[nameEnd+1:]
			valueEnd := strings.Index(source, "</parameter>")
			if valueEnd < 0 {
				return fmt.Errorf("incomplete parameter value")
			}
			value := strings.TrimSuffix(strings.TrimPrefix(source[:valueEnd], "\n"), "\n")
			if _, exists := function.Arguments[name]; exists {
				return fmt.Errorf("duplicate tool parameter %q", name)
			}
			property, exists := tool.InputSchema.Properties[name]
			if !exists {
				return fmt.Errorf("unknown tool parameter %q", name)
			}
			if property.Type == JSONSchemaTypeString {
				function.Arguments[name] = value
			} else {
				var parsed any
				if err := json.UnmarshalString(value, &parsed); err != nil {
					return fmt.Errorf("invalid parameter %s JSON: %w", name, err)
				}
				function.Arguments[name] = parsed
			}
			source = strings.TrimSpace(source[valueEnd+len("</parameter>"):])
		}
		if strings.TrimSpace(strings.TrimPrefix(source, "</function>")) != "" {
			return fmt.Errorf("unexpected content after function")
		}
	}
	if _, ok := p.tools[function.Name]; !ok || function.Arguments == nil {
		return fmt.Errorf("unknown tool or invalid argument object")
	}
	input, err := json.Marshal(function.Arguments)
	if err != nil {
		return err
	}
	call := conversation.ToolUseBlock{ID: uuid.NewString(), Name: function.Name, Input: shared.RawJSON(input)}
	index := len(p.content)
	p.content = append(p.content, call)
	p.calls++
	if p.handler != nil {
		for _, event := range []ModelCallStreamEvent{
			{Type: ModelCallStreamEventToolUseStart, Index: index, ToolUseID: call.ID, ToolName: call.Name},
			{Type: ModelCallStreamEventToolInputDelta, Index: index, ToolUseID: call.ID, Delta: string(input)},
			{Type: ModelCallStreamEventToolUseDone, Index: index, ToolUseID: call.ID, ToolName: call.Name, ToolInput: call.Input},
		} {
			if err := p.handler(event); err != nil {
				return err
			}
		}
	}
	return nil
}
