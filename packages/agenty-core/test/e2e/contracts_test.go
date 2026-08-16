//go:build e2e

package e2e_test

import (
	"encoding/json"
	"time"
)

const (
	errParse             = -32700
	errInvalidRequest    = -32600
	errMethodMissing     = -32601
	errInvalidParams     = -32602
	errNotFound          = -32001
	errAlreadyExists     = -32002
	errMessageTooLarge   = -32003
	errChunkPayloadLarge = -32004
)

var publicRPCMethods = []string{
	"initialize.already",
	"initialize.complete",
	"agent.create",
	"agent.get",
	"agent.list",
	"agent.update",
	"agent.delete",
	"provider.create",
	"provider.get",
	"provider.list",
	"provider.update",
	"provider.delete",
	"provider.addModel",
	"provider.removeModel",
	"session.create",
	"session.get",
	"session.list",
	"session.delete",
	"session.setTitle",
	"session.setModel",
	"session.setReasoningEffort",
	"session.setCwd",
	"session.start",
	"session.stop",
	"chunk.begin",
	"chunk.part",
	"chunk.commit",
	"chunk.abort",
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type rpcNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return e.Message
}

type ModelRef struct {
	ProviderSlug string `json:"providerSlug"`
	ModelSlug    string `json:"modelSlug"`
}

type InitializeResult struct {
	Initialized bool `json:"initialized"`
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

type Agent struct {
	Slug                   string         `json:"slug"`
	Name                   string         `json:"name"`
	Description            string         `json:"description"`
	Soul                   string         `json:"soul"`
	DefaultModel           *ModelRef      `json:"defaultModel"`
	DefaultContextWindow   int64          `json:"defaultContextWindow"`
	DefaultReasoningEffort string         `json:"defaultReasoningEffort"`
	IsDefault              bool           `json:"isDefault"`
	Metadata               map[string]any `json:"metadata"`
	CreatedAt              time.Time      `json:"createdAt"`
	UpdatedAt              time.Time      `json:"updatedAt"`
}

type Model struct {
	Slug                   string            `json:"slug"`
	Name                   string            `json:"name"`
	ContextWindow          int               `json:"contextWindow"`
	MaxOutputTokens        int64             `json:"maxOutputTokens"`
	MultiModal             bool              `json:"multiModal"`
	Light                  bool              `json:"light"`
	ReasoningEffortMapping map[string]string `json:"reasoningEffortMapping"`
	IsDefault              bool              `json:"isDefault"`
}

type Provider struct {
	Slug     string         `json:"slug"`
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	BaseURL  string         `json:"baseUrl"`
	APIKey   string         `json:"apiKey"`
	Models   []Model        `json:"models"`
	Metadata map[string]any `json:"metadata"`
}

type Session struct {
	ID                     string    `json:"id"`
	AgentSlug              string    `json:"agentSlug"`
	Title                  *string   `json:"title"`
	Cwd                    *string   `json:"cwd"`
	CurrentModel           *ModelRef `json:"currentModel"`
	ContextWindow          int64     `json:"contextWindow"`
	CurrentReasoningEffort string    `json:"currentReasoningEffort"`
	Rounds                 []Round   `json:"rounds"`
	CreatedAt              time.Time `json:"createdAt"`
	UpdatedAt              time.Time `json:"updatedAt"`
}

type SessionSummary struct {
	ID                  string `json:"id"`
	Title               string `json:"title"`
	AgentSlug           string `json:"agentSlug"`
	LastProviderSlug    string `json:"lastProviderSlug"`
	LastModelSlug       string `json:"lastModelSlug"`
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
	Slug    string `json:"slug,omitempty"`
	ID      string `json:"id,omitempty"`
	Deleted bool   `json:"deleted"`
}

type AgentCreateInput struct {
	Slug                   string         `json:"slug"`
	Name                   string         `json:"name"`
	Description            string         `json:"description,omitempty"`
	Soul                   string         `json:"soul,omitempty"`
	DefaultModel           *ModelRef      `json:"defaultModel,omitempty"`
	DefaultContextWindow   int64          `json:"defaultContextWindow,omitempty"`
	DefaultReasoningEffort string         `json:"defaultReasoningEffort,omitempty"`
	IsDefault              bool           `json:"isDefault,omitempty"`
	Metadata               map[string]any `json:"metadata,omitempty"`
}

type AgentUpdateInput struct {
	Slug        string         `json:"slug"`
	Name        *string        `json:"name,omitempty"`
	Description *string        `json:"description,omitempty"`
	Soul        *string        `json:"soul,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type ProviderCreateInput struct {
	Slug     string         `json:"slug"`
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	BaseURL  string         `json:"baseUrl,omitempty"`
	APIKey   string         `json:"apiKey,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type ProviderUpdateInput struct {
	Slug     string         `json:"slug"`
	Name     *string        `json:"name,omitempty"`
	Type     *string        `json:"type,omitempty"`
	BaseURL  *string        `json:"baseUrl,omitempty"`
	APIKey   *string        `json:"apiKey,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type ModelInput struct {
	ProviderSlug           string            `json:"providerSlug"`
	ModelSlug              string            `json:"modelSlug"`
	Name                   string            `json:"name"`
	ContextWindow          int               `json:"contextWindow,omitempty"`
	MaxOutputTokens        int64             `json:"maxOutputTokens"`
	MultiModal             bool              `json:"multiModal,omitempty"`
	Light                  bool              `json:"light,omitempty"`
	ReasoningEffortMapping map[string]string `json:"reasoningEffortMapping,omitempty"`
	IsDefault              bool              `json:"isDefault,omitempty"`
}

type SessionCreateInput struct {
	AgentSlug       string  `json:"agentSlug"`
	ProviderSlug    string  `json:"providerSlug"`
	ModelSlug       string  `json:"modelSlug"`
	ContextWindow   int64   `json:"contextWindow,omitempty"`
	ReasoningEffort string  `json:"reasoningEffort,omitempty"`
	Cwd             *string `json:"cwd,omitempty"`
}

type SessionListInput struct {
	AgentSlug string `json:"agentSlug,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

type ContentInput struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`
	URL      string `json:"url,omitempty"`
}
