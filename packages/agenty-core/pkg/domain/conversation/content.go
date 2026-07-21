package conversation

import (
	"fmt"

	json "github.com/bytedance/sonic"

	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

type BlockType string

const (
	BlockText       BlockType = "text"
	BlockThinking   BlockType = "thinking"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
	BlockImage      BlockType = "image"
)

type ContentBlock interface {
	BlockType() BlockType
}

type Content []ContentBlock

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

type ThinkingBlock struct {
	Thinking  string         `json:"thinking"`
	Signature string         `json:"signature,omitempty"`
	Redacted  bool           `json:"redacted,omitempty"`
	Extra     shared.RawJSON `json:"extra,omitempty"`
}

func (ThinkingBlock) BlockType() BlockType {
	return BlockThinking
}

func (b ThinkingBlock) MarshalJSON() ([]byte, error) {
	type alias ThinkingBlock
	return json.Marshal(struct {
		Type BlockType `json:"type"`
		alias
	}{Type: BlockThinking, alias: alias(b)})
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
		*c = nil
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
		case BlockThinking:
			var b ThinkingBlock
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
