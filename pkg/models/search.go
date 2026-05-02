/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package models

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type SearchContext struct {
	AgentID   uuid.UUID
	SessionID uuid.UUID
	ModelID   uuid.UUID
	ModelCode string
	Cwd       string
}

type SearchSpec struct {
	ID           string   `json:"id"`
	Channel      string   `json:"channel"`
	Query        string   `json:"query"`
	Limit        int      `json:"limit"`
	IncludeGlobs []string `json:"includeGlobs"`
	ExcludeGlobs []string `json:"excludeGlobs"`
}

type SearchRequest struct {
	Searches []SearchSpec `json:"searches"`
}

func ValidateSearchRequest(req SearchRequest) error {
	if len(req.Searches) == 0 {
		return fmt.Errorf("searches array cannot be empty")
	}

	currentIDs := make(map[string]struct{})
	for _, spec := range req.Searches {
		if _, exists := currentIDs[spec.ID]; exists {
			return fmt.Errorf("duplicate search spec id: %q", spec.ID)
		}
		currentIDs[spec.ID] = struct{}{}
		if strings.TrimSpace(spec.Query) == "" {
			return fmt.Errorf("search spec %q has empty query", spec.ID)
		}
		switch strings.TrimSpace(spec.Channel) {
		case "knowledge_base", "workspace_files", "web_search":
		default:
			return fmt.Errorf("unknown channel %q in spec %q", spec.Channel, spec.ID)
		}
	}
	return nil
}

type KBSearchChannelResult struct {
	SearchID   string  `json:"searchId"`
	ItemID     string  `json:"itemId"`
	ItemTitle  string  `json:"itemTitle,omitempty"`
	Category   string  `json:"category"`
	ChunkIndex int     `json:"chunkIndex"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
}

type WebSearchChannelResult struct {
	SearchID string  `json:"searchId"`
	Title    string  `json:"title"`
	URL      string  `json:"url"`
	Content  string  `json:"content"`
	Score    float64 `json:"score,omitempty"`
}

type WorkspaceFileSearchResult struct {
	SearchID     string  `json:"searchId"`
	Path         string  `json:"path"`
	RelativePath string  `json:"relativePath"`
	StartLine    int     `json:"startLine"`
	EndLine      int     `json:"endLine"`
	Content      string  `json:"content"`
	Score        float64 `json:"score"`
}

type RankedSearchResult struct {
	ID           string  `json:"id"`
	SearchID     string  `json:"searchId"`
	Channel      string  `json:"channel"`
	Ranker       string  `json:"ranker"`
	Title        string  `json:"title,omitempty"`
	URL          string  `json:"url,omitempty"`
	Path         string  `json:"path,omitempty"`
	RelativePath string  `json:"relativePath,omitempty"`
	StartLine    int     `json:"startLine,omitempty"`
	EndLine      int     `json:"endLine,omitempty"`
	ItemID       string  `json:"itemId,omitempty"`
	ItemTitle    string  `json:"itemTitle,omitempty"`
	Category     string  `json:"category,omitempty"`
	ChunkIndex   int     `json:"chunkIndex,omitempty"`
	Content      string  `json:"content"`
	RawScore     float64 `json:"rawScore,omitempty"`
	FusedScore   float64 `json:"fusedScore"`
	RerankScore  float64 `json:"rerankScore,omitempty"`
}

type SearchChannelResponse struct {
	Results     any               `json:"results"`
	QueriesUsed map[string]string `json:"queriesUsed"`
	Quality     string            `json:"quality"`
	Message     string            `json:"message"`
	Errors      []string          `json:"errors,omitempty"`
}

type SearchResponse struct {
	KnowledgeBase  *SearchChannelResponse `json:"knowledgeBase,omitempty"`
	WorkspaceFiles *SearchChannelResponse `json:"workspaceFiles,omitempty"`
	WebSearch      *SearchChannelResponse `json:"webSearch,omitempty"`
	RankedResults  []RankedSearchResult   `json:"rankedResults,omitempty"`
	OverallQuality string                 `json:"overallQuality"`
	EvaluatorNotes string                 `json:"evaluatorNotes,omitempty"`
}

type SearchRerankCandidate struct {
	ID       string  `json:"id"`
	Channel  string  `json:"channel"`
	Title    string  `json:"title,omitempty"`
	Source   string  `json:"source,omitempty"`
	Content  string  `json:"content"`
	RRFScore float64 `json:"rrfScore"`
	RawScore float64 `json:"rawScore,omitempty"`
}

type SearchFusionRerankResult struct {
	OrderedIDs []string           `json:"rankedIds"`
	Scores     map[string]float64 `json:"scores"`
	Summary    string             `json:"summary"`
}

type SearchCandidateSet struct {
	Weight     float64
	Candidates []RankedSearchResult
}
