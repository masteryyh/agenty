package modelcall

import (
	"strings"
	"testing"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	local "github.com/masteryyh/agenty-core/pkg/domain/localinference"
)

func TestLocalParserAcrossEveryByteBoundary(t *testing.T) {
	tools := []ToolDefinition{{Name: "echo", InputSchema: JSONSchema{Type: JSONSchemaTypeObject, Properties: map[string]JSONSchema{"text": {Type: JSONSchemaTypeString}, "count": {Type: JSONSchemaTypeInteger}}}}}
	for _, test := range []struct {
		format, source string
		prefilled      bool
	}{
		{"qwen3", "<think>思考😀</think>正文<tool_call>{\"name\":\"echo\",\"arguments\":{\"text\":\"你好😀\",\"count\":2}}</tool_call>", false},
		{"qwen3_5", "思考😀</think>正文<tool_call>\n<function=echo>\n<parameter=text>\n你好😀\n</parameter>\n<parameter=count>\n2\n</parameter>\n</function>\n</tool_call>", true},
	} {
		t.Run(test.format, func(t *testing.T) {
			for boundary := 0; boundary <= len(test.source); boundary++ {
				var text, reasoning strings.Builder
				p := newLocalParser(test.format, tools, func(event ModelCallStreamEvent) error {
					if event.Type == ModelCallStreamEventTextDelta {
						text.WriteString(event.Delta)
					}
					if event.Type == ModelCallStreamEventReasoningDelta {
						reasoning.WriteString(event.Delta)
					}
					return nil
				})
				if test.prefilled {
					p.mode = "reasoning"
				}
				if err := p.push(test.source[:boundary], false); err != nil {
					t.Fatal(boundary, err)
				}
				if err := p.push(test.source[boundary:], true); err != nil {
					t.Fatal(boundary, err)
				}
				if text.String() != "正文" || reasoning.String() != "思考😀" || p.calls != 1 {
					t.Fatalf("boundary %d: %q %q calls=%d", boundary, text.String(), reasoning.String(), p.calls)
				}
				call := p.content[len(p.content)-1].(conversation.ToolUseBlock)
				args, err := rawObject(call.Input, "test")
				if err != nil {
					t.Fatal(err)
				}
				if args["text"] != "你好😀" {
					t.Fatalf("corrupted arguments: %s", call.Input)
				}
			}
		})
	}
}

func TestLocalParserRejectsUnknownAndIncompleteCalls(t *testing.T) {
	for _, source := range []string{"<tool_call>", "<tool_call>{\"name\":\"unknown\",\"arguments\":{}}</tool_call>", "<tool_call>{broken}</tool_call>"} {
		if err := newLocalParser("qwen3", nil, nil).push(source, true); err == nil {
			t.Fatalf("accepted %q", source)
		}
	}
}

func TestLocalRequestPreservesToolsAndMapsThinking(t *testing.T) {
	model := ModelCallConfig{APIType: APILocal, Local: &local.Model{OutputParser: "qwen3_5", ThinkingEffort: "medium", Resources: []local.Resource{{ID: "r", Backend: local.MLX, Format: "safetensors", WeightsPath: "/model", CodecDirectory: "/model"}}}}
	request := ModelCallRequest{SystemPrompt: "system", Messages: []ModelCallMessage{{Role: conversation.RoleUser, Content: conversation.Text("你好")}}, Tools: []ToolDefinition{{Name: "z"}, {Name: "a"}}}
	rendered, err := localRequest(model, request)
	if err != nil {
		t.Fatal(err)
	}
	if rendered.TemplateKwargs["enable_thinking"] != false || len(rendered.Tools) != 2 || rendered.Tools[0].(map[string]any)["function"].(map[string]any)["name"] != "z" {
		t.Fatalf("unexpected render request: %+v", rendered)
	}
	request.ReasoningEffort = "high"
	if _, err := localRequest(model, request); err == nil {
		t.Fatal("unsupported thinking grade accepted")
	}
	request.ReasoningEffort = "medium"
	rendered, err = localRequest(model, request)
	if err != nil || rendered.TemplateKwargs["enable_thinking"] != true {
		t.Fatal(rendered, err)
	}
	request.OutputFormat = &OutputFormat{}
	if _, err := localRequest(model, request); err == nil {
		t.Fatal("unconstrained output accepted as schema-constrained")
	}
}
