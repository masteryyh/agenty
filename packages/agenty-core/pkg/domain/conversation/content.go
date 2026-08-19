package conversation

import (
	"fmt"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type BlockType string

const (
	BlockText        BlockType = "text"
	BlockReasoning   BlockType = "reasoning"
	BlockToolUse     BlockType = "tool_use"
	BlockToolResult  BlockType = "tool_result"
	BlockImage       BlockType = "image"
	BlockShellCall   BlockType = "shell_call"
	BlockShellOutput BlockType = "shell_call_output"
)

type ContentBlock interface {
	BlockType() BlockType
}

type Content []ContentBlock

// MarshalJSON keeps the public content contract stable for empty messages and
// nested tool results. JSON arrays must not regress to null when the in-memory
// slice is nil.
func (c Content) MarshalJSON() ([]byte, error) {
	if c == nil {
		return []byte("[]"), nil
	}
	type contentAlias Content
	return json.Marshal(contentAlias(c))
}

type TextBlock struct {
	Text string `json:"text"`
}

func (TextBlock) BlockType() BlockType {
	return BlockText
}

func (b TextBlock) MarshalJSON() ([]byte, error) {
	type alias TextBlock
	return json.Marshal(struct {
		Type BlockType `json:"type"`
		alias
	}{Type: BlockText, alias: alias(b)})
}

type ReasoningBlock struct {
	Reasoning string         `json:"reasoning"`
	Signature string         `json:"signature,omitempty"`
	Redacted  bool           `json:"redacted,omitempty"`
	Extra     shared.RawJSON `json:"extra,omitempty"`
}

func (ReasoningBlock) BlockType() BlockType {
	return BlockReasoning
}

func (b ReasoningBlock) MarshalJSON() ([]byte, error) {
	type alias ReasoningBlock
	return json.Marshal(struct {
		Type BlockType `json:"type"`
		alias
	}{Type: BlockReasoning, alias: alias(b)})
}

type ToolUseBlock struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input shared.RawJSON `json:"input"`
}

func (ToolUseBlock) BlockType() BlockType {
	return BlockToolUse
}

func (b ToolUseBlock) MarshalJSON() ([]byte, error) {
	type alias ToolUseBlock
	return json.Marshal(struct {
		Type BlockType `json:"type"`
		alias
	}{Type: BlockToolUse, alias: alias(b)})
}

type ToolResultBlock struct {
	ToolUseID string  `json:"toolUseId"`
	Content   Content `json:"content"`
	IsError   bool    `json:"isError,omitempty"`
}

func (ToolResultBlock) BlockType() BlockType {
	return BlockToolResult
}

func (b ToolResultBlock) MarshalJSON() ([]byte, error) {
	type alias ToolResultBlock
	return json.Marshal(struct {
		Type BlockType `json:"type"`
		alias
	}{Type: BlockToolResult, alias: alias(b)})
}

type ShellCallBlock struct {
	ID              string   `json:"id,omitempty"`
	CallID          string   `json:"callId"`
	Commands        []string `json:"commands"`
	TimeoutMs       int64    `json:"timeoutMs,omitempty"`
	MaxOutputLength int64    `json:"maxOutputLength,omitempty"`
}

func (b ShellCallBlock) ToolUseBlock() ToolUseBlock {
	input, _ := json.Marshal(struct {
		Commands        []string `json:"commands"`
		TimeoutMs       int64    `json:"timeout_ms,omitempty"`
		MaxOutputLength int64    `json:"max_output_length,omitempty"`
	}{
		Commands: b.Commands, TimeoutMs: b.TimeoutMs, MaxOutputLength: b.MaxOutputLength,
	})
	return ToolUseBlock{ID: b.CallID, Name: "shell", Input: shared.RawJSON(input)}
}

func (ShellCallBlock) BlockType() BlockType {
	return BlockShellCall
}

func (b ShellCallBlock) MarshalJSON() ([]byte, error) {
	type alias ShellCallBlock
	return json.Marshal(struct {
		Type BlockType `json:"type"`
		alias
	}{Type: BlockShellCall, alias: alias(b)})
}

type ShellOutcome struct {
	Type     string `json:"type"`
	ExitCode *int64 `json:"exitCode,omitempty"`
}

type ShellCommandOutput struct {
	Stdout  string       `json:"stdout"`
	Stderr  string       `json:"stderr"`
	Outcome ShellOutcome `json:"outcome"`
}

type ShellCallOutputBlock struct {
	CallID          string               `json:"callId"`
	MaxOutputLength int64                `json:"maxOutputLength"`
	Output          []ShellCommandOutput `json:"output"`
}

func (ShellCallOutputBlock) BlockType() BlockType {
	return BlockShellOutput
}

func (b ShellCallOutputBlock) MarshalJSON() ([]byte, error) {
	type alias ShellCallOutputBlock
	return json.Marshal(struct {
		Type BlockType `json:"type"`
		alias
	}{Type: BlockShellOutput, alias: alias(b)})
}

type ImageBlock struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data,omitempty"`
	URI      string `json:"uri,omitempty"`
}

func (ImageBlock) BlockType() BlockType {
	return BlockImage
}

func (b ImageBlock) MarshalJSON() ([]byte, error) {
	type alias ImageBlock
	return json.Marshal(struct {
		Type BlockType `json:"type"`
		alias
	}{Type: BlockImage, alias: alias(b)})
}

func (c *Content) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*c = make(Content, 0)
		return nil
	}

	var raws []shared.RawJSON
	if err := json.Unmarshal(data, &raws); err != nil {
		return err
	}

	blocks := make(Content, 0, len(raws))
	for _, raw := range raws {
		var head struct {
			Type BlockType `json:"type"`
		}
		if err := json.Unmarshal(raw, &head); err != nil {
			return err
		}

		var block ContentBlock
		switch head.Type {
		case BlockText:
			var b TextBlock
			if err := json.Unmarshal(raw, &b); err != nil {
				return err
			}
			block = b
		case BlockReasoning:
			var b ReasoningBlock
			if err := json.Unmarshal(raw, &b); err != nil {
				return err
			}
			block = b
		case BlockToolUse:
			var b ToolUseBlock
			if err := json.Unmarshal(raw, &b); err != nil {
				return err
			}
			block = b
		case BlockToolResult:
			var b ToolResultBlock
			if err := json.Unmarshal(raw, &b); err != nil {
				return err
			}
			block = b
		case BlockShellCall:
			var b ShellCallBlock
			if err := json.Unmarshal(raw, &b); err != nil {
				return err
			}
			block = b
		case BlockShellOutput:
			var b ShellCallOutputBlock
			if err := json.Unmarshal(raw, &b); err != nil {
				return err
			}
			block = b
		case BlockImage:
			var b ImageBlock
			if err := json.Unmarshal(raw, &b); err != nil {
				return err
			}
			block = b
		default:
			return fmt.Errorf("conversation: unknown content block type %q", head.Type)
		}
		blocks = append(blocks, block)
	}

	*c = blocks
	return nil
}

func Text(text string) Content {
	return Content{TextBlock{Text: text}}
}
