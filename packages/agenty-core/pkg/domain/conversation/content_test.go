package conversation

import (
	"testing"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestContentRoundTrip(t *testing.T) {
	original := Content{
		TextBlock{Text: "hello"},
		ReasoningBlock{Reasoning: "analyze the request", Signature: "sig123", Extra: shared.RawJSON(`{"provider":"anthropic"}`)},
		ToolUseBlock{ID: "call_1", Name: "read_file", Input: shared.RawJSON(`{"path":"/tmp/x"}`)},
		ToolResultBlock{ToolUseID: "call_1", Content: Content{TextBlock{Text: "file contents"}}},
		ImageBlock{MimeType: "image/png", Data: "aGVsbG8="},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Content
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(decoded) != len(original) {
		t.Fatalf("length mismatch: got %d, want %d", len(decoded), len(original))
	}

	if block, ok := decoded[0].(TextBlock); !ok {
		t.Errorf("block 0 = %T, want TextBlock", decoded[0])
	} else if block.Text != "hello" {
		t.Errorf("text = %q, want hello", block.Text)
	}
	if block, ok := decoded[1].(ReasoningBlock); !ok {
		t.Errorf("block 1 = %T, want ReasoningBlock", decoded[1])
	} else if block.Reasoning != "analyze the request" || block.Signature != "sig123" || string(block.Extra) != `{"provider":"anthropic"}` {
		t.Errorf("reasoning block = %+v", block)
	}
	if block, ok := decoded[2].(ToolUseBlock); !ok {
		t.Errorf("block 2 = %T, want ToolUseBlock", decoded[2])
	} else if block.ID != "call_1" || block.Name != "read_file" || string(block.Input) != `{"path":"/tmp/x"}` {
		t.Errorf("tool use block = %+v", block)
	}
	if tr, ok := decoded[3].(ToolResultBlock); !ok {
		t.Errorf("block 3 = %T, want ToolResultBlock", decoded[3])
	} else {
		if tr.ToolUseID != "call_1" {
			t.Errorf("tool use id = %q, want call_1", tr.ToolUseID)
		}
		if len(tr.Content) != 1 {
			t.Errorf("nested content length = %d, want 1", len(tr.Content))
		} else if _, ok := tr.Content[0].(TextBlock); !ok {
			t.Errorf("nested block = %T, want TextBlock", tr.Content[0])
		}
	}
	if block, ok := decoded[4].(ImageBlock); !ok {
		t.Errorf("block 4 = %T, want ImageBlock", decoded[4])
	} else if block.MimeType != "image/png" || block.Data != "aGVsbG8=" {
		t.Errorf("image block = %+v", block)
	}

	// Re-marshaling the decoded content must reproduce the original bytes.
	redata, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if string(redata) != string(data) {
		t.Errorf("round-trip mismatch:\n first: %s\nsecond: %s", data, redata)
	}
}

func TestContentUnmarshalUnknownType(t *testing.T) {
	var c Content
	err := json.Unmarshal([]byte(`[{"type":"video","url":"x"}]`), &c)
	if err == nil {
		t.Fatal("expected error for unknown block type, got nil")
	}
}

func TestShellBlocksRoundTrip(t *testing.T) {
	exitCode := int64(7)
	original := Content{
		ShellCallBlock{
			ID: "sh_1", CallID: "call_1", Commands: []string{"pwd", "false"},
			TimeoutMs: 1000, MaxOutputLength: 4096,
		},
		ToolResultBlock{
			ToolUseID: "call_1",
			Content: Content{ShellCallOutputBlock{
				CallID: "call_1", MaxOutputLength: 4096, OpenAINative: boolPointer(true),
				Output: []ShellCommandOutput{
					{Stdout: "/tmp\n", Outcome: ShellOutcome{Type: "exit", ExitCode: int64Pointer(0)}},
					{Stderr: "failed\n", Outcome: ShellOutcome{Type: "exit", ExitCode: &exitCode}},
				},
			}},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Content
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded[0].(ShellCallBlock); !ok {
		t.Fatalf("call block = %T, want ShellCallBlock", decoded[0])
	}
	result, ok := decoded[1].(ToolResultBlock)
	if !ok || len(result.Content) != 1 {
		t.Fatalf("result block = %#v", decoded[1])
	}
	if output, ok := result.Content[0].(ShellCallOutputBlock); !ok || len(output.Output) != 2 ||
		output.OpenAINative == nil || !*output.OpenAINative {
		t.Fatalf("shell output = %#v", result.Content)
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}

func boolPointer(value bool) *bool {
	return &value
}

func TestContentUnmarshalNull(t *testing.T) {
	c := Content{TextBlock{Text: "x"}}
	if err := json.Unmarshal([]byte(`null`), &c); err != nil {
		t.Fatalf("unmarshal null: %v", err)
	}
	if c == nil || len(c) != 0 {
		t.Errorf("expected empty content, got %v", c)
	}
}

func TestContentNilMarshalsAsArray(t *testing.T) {
	data, err := json.Marshal(Content(nil))
	if err != nil {
		t.Fatalf("marshal nil content: %v", err)
	}
	if string(data) != "[]" {
		t.Fatalf("nil content = %s, want []", data)
	}
}
