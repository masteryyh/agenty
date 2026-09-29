//go:build e2e

package e2e_test

import (
	"encoding/json"
	"time"
)

type ModelRef struct {
	ProviderCode string `json:"providerCode"`
	ModelCode    string `json:"modelCode"`
}

type InitializeResult struct {
	Initialized            bool      `json:"initialized"`
	DefaultModel           *ModelRef `json:"defaultModel,omitempty"`
	DefaultReasoningEffort string    `json:"defaultReasoningEffort,omitempty"`
}

type SessionEvent struct {
	Type      string       `json:"type"`
	SessionID string       `json:"sessionId"`
	RoundID   string       `json:"roundId"`
	Sequence  uint64       `json:"sequence"`
	Iteration int          `json:"iteration"`
	Stream    *StreamEvent `json:"stream"`
	Message   *Message     `json:"message"`
	Status    string       `json:"status"`
	Usage     *TokenUsage  `json:"usage"`
	Error     *string      `json:"error"`
}

type StreamEvent struct {
	Type      string          `json:"type"`
	Index     int             `json:"index"`
	Delta     string          `json:"delta"`
	ToolUseID string          `json:"toolUseId"`
	ToolName  string          `json:"toolName"`
	ToolInput json.RawMessage `json:"toolInput"`
}

type Model struct {
	Code             string   `json:"code"`
	Name             string   `json:"name"`
	ContextWindow    int      `json:"contextWindow"`
	MaxOutputTokens  int64    `json:"maxOutputTokens"`
	MultiModal       bool     `json:"multiModal"`
	Light            bool     `json:"light"`
	ReasoningEfforts []string `json:"reasoningEfforts"`
	IsDefault        bool     `json:"isDefault"`
}

type AvailableModel struct {
	Code             string   `json:"code"`
	Name             string   `json:"name"`
	ContextWindow    int      `json:"contextWindow"`
	MaxOutputTokens  int64    `json:"maxOutputTokens"`
	MultiModal       bool     `json:"multiModal"`
	ReasoningEfforts []string `json:"reasoningEfforts"`
}

type Provider struct {
	Code          string         `json:"code"`
	Name          string         `json:"name"`
	Type          string         `json:"type"`
	BaseURL       string         `json:"baseUrl"`
	APIKey        string         `json:"apiKey"`
	Builtin       bool           `json:"builtin"`
	Official      bool           `json:"official"`
	ModelsURL     string         `json:"modelsUrl"`
	TokenCountURL string         `json:"tokenCountUrl"`
	Models        []Model        `json:"models"`
	Metadata      map[string]any `json:"metadata"`
}

type Session struct {
	ID                     string    `json:"id"`
	Title                  *string   `json:"title"`
	Cwd                    *string   `json:"cwd"`
	CurrentModel           *ModelRef `json:"currentModel"`
	ContextWindow          int64     `json:"contextWindow"`
	CurrentReasoningEffort string    `json:"currentReasoningEffort"`
	ToolDialect            string    `json:"toolDialect"`
	Rounds                 []Round   `json:"rounds"`
	CreatedAt              time.Time `json:"createdAt"`
	UpdatedAt              time.Time `json:"updatedAt"`
}

type SessionSummary struct {
	ID                  string `json:"id"`
	Title               string `json:"title"`
	LastProviderCode    string `json:"lastProviderCode"`
	LastModelCode       string `json:"lastModelCode"`
	ContextWindow       int64  `json:"contextWindow"`
	LastReasoningEffort string `json:"lastReasoningEffort"`
}

type Round struct {
	ID              string     `json:"id"`
	SessionID       string     `json:"sessionId"`
	Sequence        int        `json:"sequence"`
	Status          string     `json:"status"`
	Model           ModelRef   `json:"model"`
	ContextWindow   int64      `json:"contextWindow"`
	ReasoningEffort string     `json:"reasoningEffort"`
	Messages        []Message  `json:"messages"`
	Usage           TokenUsage `json:"usage"`
	Error           *string    `json:"error"`
}

type Message struct {
	ID      string         `json:"id"`
	RoundID string         `json:"roundId"`
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
	Usage   *TokenUsage    `json:"usage"`
}

type ContentBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	MimeType  string `json:"mimeType"`
	Data      string `json:"data"`
	URL       string `json:"url"`
	ToolUseID string `json:"toolUseId"`
	IsError   bool   `json:"isError"`
}

type TokenUsage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CachedRead int64 `json:"cachedRead"`
	CacheWrite int64 `json:"cacheWrite"`
	Reasoning  int64 `json:"reasoning"`
	Total      int64 `json:"total"`
}

type ExecutionStart struct {
	SessionID string `json:"sessionId"`
	RoundID   string `json:"roundId"`
	Status    string `json:"status"`
}

type ExecutionStop struct {
	SessionID     string `json:"sessionId"`
	RoundID       string `json:"roundId"`
	StopRequested bool   `json:"stopRequested"`
}

type DeleteResult struct {
	Code    string `json:"code,omitempty"`
	ID      string `json:"id,omitempty"`
	Deleted bool   `json:"deleted"`
}

type ProviderCreateInput struct {
	Code     string         `json:"code"`
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	BaseURL  string         `json:"baseUrl,omitempty"`
	APIKey   string         `json:"apiKey,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type ProviderUpdateInput struct {
	Code     string         `json:"code"`
	Name     *string        `json:"name,omitempty"`
	Type     *string        `json:"type,omitempty"`
	BaseURL  *string        `json:"baseUrl,omitempty"`
	APIKey   *string        `json:"apiKey,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type ModelInput struct {
	ProviderCode    string `json:"providerCode"`
	ModelCode       string `json:"modelCode"`
	Name            string `json:"name"`
	ContextWindow   int    `json:"contextWindow,omitempty"`
	MaxOutputTokens int64  `json:"maxOutputTokens"`
	MultiModal      bool   `json:"multiModal,omitempty"`
	Light           bool   `json:"light,omitempty"`
	Reasoning       *bool  `json:"reasoning,omitempty"`
	IsDefault       bool   `json:"isDefault,omitempty"`
}

type SessionCreateInput struct {
	ProviderCode    string  `json:"providerCode"`
	ModelCode       string  `json:"modelCode"`
	ContextWindow   int64   `json:"contextWindow,omitempty"`
	ReasoningEffort string  `json:"reasoningEffort,omitempty"`
	Cwd             *string `json:"cwd,omitempty"`
}

type SessionListInput struct {
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

type ContentInput struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`
	URL      string `json:"url,omitempty"`
}
