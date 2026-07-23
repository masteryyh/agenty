package conversation

import (
	"testing"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

func TestContentRoundTrip(t *testing.T) {
	original := Content{
		TextBlock{Text: "hello"},
		ThinkingBlock{Thinking: "let me think", Signature: "sig123", Extra: shared.RawJSON(`{"provider":"anthropic"}`)},
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
	if block, ok := decoded[1].(ThinkingBlock); !ok {
		t.Errorf("block 1 = %T, want ThinkingBlock", decoded[1])
	} else if block.Thinking != "let me think" || block.Signature != "sig123" || string(block.Extra) != `{"provider":"anthropic"}` {
		t.Errorf("thinking block = %+v", block)
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

func TestContentUnmarshalNull(t *testing.T) {
	c := Content{TextBlock{Text: "x"}}
	if err := json.Unmarshal([]byte(`null`), &c); err != nil {
		t.Fatalf("unmarshal null: %v", err)
	}
	if c != nil {
		t.Errorf("expected nil content, got %v", c)
	}
}
