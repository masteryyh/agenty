//go:build e2e

package e2e_test

import "time"

const (
	errParse          = -32700
	errInvalidRequest = -32600
	errMethodMissing  = -32601
	errInvalidParams  = -32602
	errNotFound       = -32001
	errAlreadyExists  = -32002
)

type modelRefView struct {
	ProviderSlug string `json:"providerSlug"`
	ModelSlug    string `json:"modelSlug"`
}

type agentView struct {
	Slug                   string         `json:"slug"`
	Name                   string         `json:"name"`
	Description            string         `json:"description"`
	Soul                   string         `json:"soul"`
	DefaultModel           *modelRefView  `json:"defaultModel"`
	DefaultContextWindow   int64          `json:"defaultContextWindow"`
	DefaultReasoningEffort string         `json:"defaultReasoningEffort"`
	IsDefault              bool           `json:"isDefault"`
	Metadata               map[string]any `json:"metadata"`
	CreatedAt              time.Time      `json:"createdAt"`
	UpdatedAt              time.Time      `json:"updatedAt"`
}

type modelView struct {
	Slug                   string            `json:"slug"`
	Name                   string            `json:"name"`
	ContextWindow          int               `json:"contextWindow"`
	MaxOutputTokens        int64             `json:"maxOutputTokens"`
	MultiModal             bool              `json:"multiModal"`
	Embedding              bool              `json:"embedding"`
	Light                  bool              `json:"light"`
	ReasoningEffortMapping map[string]string `json:"reasoningEffortMapping"`
	IsDefault              bool              `json:"isDefault"`
}

type providerView struct {
	Slug     string         `json:"slug"`
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	BaseURL  string         `json:"baseUrl"`
	APIKey   string         `json:"apiKey"`
	Models   []modelView    `json:"models"`
	Metadata map[string]any `json:"metadata"`
}

type sessionView struct {
	ID                     string        `json:"id"`
	AgentSlug              string        `json:"agentSlug"`
	Title                  *string       `json:"title"`
	Cwd                    *string       `json:"cwd"`
	CurrentModel           *modelRefView `json:"currentModel"`
	ContextWindow          int64         `json:"contextWindow"`
	CurrentReasoningEffort string        `json:"currentReasoningEffort"`
	Rounds                 []roundView   `json:"rounds"`
	CreatedAt              time.Time     `json:"createdAt"`
	UpdatedAt              time.Time     `json:"updatedAt"`
}

type tokenUsageView struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CachedRead int64 `json:"cachedRead"`
	CacheWrite int64 `json:"cacheWrite"`
	Reasoning  int64 `json:"reasoning"`
	Total      int64 `json:"total"`
}

type contentBlockView struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	ToolUseID string `json:"toolUseId"`
	IsError   bool   `json:"isError"`
}

type messageView struct {
	ID      string             `json:"id"`
	RoundID string             `json:"roundId"`
	Role    string             `json:"role"`
	Content []contentBlockView `json:"content"`
	Usage   *tokenUsageView    `json:"usage"`
}

type roundView struct {
	ID              string         `json:"id"`
	SessionID       string         `json:"sessionId"`
	Sequence        int            `json:"sequence"`
	Status          string         `json:"status"`
	Model           modelRefView   `json:"model"`
	ContextWindow   int64          `json:"contextWindow"`
	ReasoningEffort string         `json:"reasoningEffort"`
	Messages        []messageView  `json:"messages"`
	Usage           tokenUsageView `json:"usage"`
	Error           *string        `json:"error"`
}

type executionStartView struct {
	SessionID string `json:"sessionId"`
	RoundID   string `json:"roundId"`
	Status    string `json:"status"`
}

type executionStopView struct {
	SessionID     string `json:"sessionId"`
	RoundID       string `json:"roundId"`
	StopRequested bool   `json:"stopRequested"`
}

type sessionSummaryView struct {
	ID                  string `json:"id"`
	Title               string `json:"title"`
	AgentSlug           string `json:"agentSlug"`
	LastProviderSlug    string `json:"lastProviderSlug"`
	LastModelSlug       string `json:"lastModelSlug"`
	ContextWindow       int64  `json:"contextWindow"`
	LastReasoningEffort string `json:"lastReasoningEffort"`
}
