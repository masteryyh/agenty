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
	Slug                  string         `json:"slug"`
	Name                  string         `json:"name"`
	Description           string         `json:"description"`
	Soul                  string         `json:"soul"`
	DefaultModel          *modelRefView  `json:"defaultModel"`
	DefaultContextWindow  int64          `json:"defaultContextWindow"`
	DefaultThinkingEffort string         `json:"defaultThinkingEffort"`
	IsDefault             bool           `json:"isDefault"`
	Metadata              map[string]any `json:"metadata"`
	CreatedAt             time.Time      `json:"createdAt"`
	UpdatedAt             time.Time      `json:"updatedAt"`
}

type modelView struct {
	Slug            string   `json:"slug"`
	Name            string   `json:"name"`
	ContextWindow   int      `json:"contextWindow"`
	MultiModal      bool     `json:"multiModal"`
	Embedding       bool     `json:"embedding"`
	Light           bool     `json:"light"`
	ThinkingEfforts []string `json:"thinkingEfforts"`
	IsDefault       bool     `json:"isDefault"`
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
	ID                    string        `json:"id"`
	AgentSlug             string        `json:"agentSlug"`
	Title                 *string       `json:"title"`
	Cwd                   *string       `json:"cwd"`
	CurrentModel          *modelRefView `json:"currentModel"`
	ContextWindow         int64         `json:"contextWindow"`
	CurrentThinkingEffort string        `json:"currentThinkingEffort"`
	CreatedAt             time.Time     `json:"createdAt"`
	UpdatedAt             time.Time     `json:"updatedAt"`
}

type sessionSummaryView struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	AgentSlug          string `json:"agentSlug"`
	LastProviderSlug   string `json:"lastProviderSlug"`
	LastModelSlug      string `json:"lastModelSlug"`
	ContextWindow      int64  `json:"contextWindow"`
	LastThinkingEffort string `json:"lastThinkingEffort"`
}
