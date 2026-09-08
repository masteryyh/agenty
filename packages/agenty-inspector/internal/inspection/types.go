package inspection

import (
	"time"

	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
	"github.com/masteryyh/agenty-core/pkg/domain/shared"
)

// SourceRef identifies a physical record; seq and message IDs are not unique
// in damaged transcripts. ContentPath addresses nested content blocks.
type SourceRef struct {
	RecordID    string `json:"recordId"`
	Line        int    `json:"line"`
	Seq         int64  `json:"seq"`
	ContentPath string `json:"contentPath,omitempty"`
}

type Record struct {
	ID         string    `json:"id"`
	Line       int       `json:"line"`
	Offset     int64     `json:"offset"`
	Length     int64     `json:"length"`
	Terminated bool      `json:"terminated"`
	Seq        int64     `json:"seq"`
	Type       string    `json:"type"`
	WroteAt    time.Time `json:"wroteAt"`
	RoundID    string    `json:"roundId,omitempty"`
	Summary    string    `json:"summary"`
	Error      string    `json:"error,omitempty"`
	Applied    bool      `json:"applied"`
}

type Diagnostic struct {
	Code     string    `json:"code"`
	Severity string    `json:"severity"`
	Message  string    `json:"message"`
	Source   SourceRef `json:"source"`
}

type SessionEntry struct {
	IssueCount int       `json:"issueCount"`
	ID         string    `json:"id"`
	SessionID  string    `json:"sessionId"`
	Path       string    `json:"path"`
	Title      string    `json:"title"`
	Agent      string    `json:"agent"`
	Model      string    `json:"model"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Size       int64     `json:"size"`
	Revision   string    `json:"revision"`
	Error      string    `json:"error,omitempty"`
}

type MessageNode struct {
	Message conversation.Message `json:"message"`
	Source  SourceRef            `json:"source"`
}

type RoundNode struct {
	ID              string                   `json:"id"`
	Sequence        int                      `json:"sequence"`
	Status          conversation.RoundStatus `json:"status"`
	Model           shared.ModelRef          `json:"model"`
	ReasoningEffort shared.ReasoningEffort   `json:"reasoningEffort"`
	ContextWindow   int64                    `json:"contextWindow"`
	Cwd             *string                  `json:"cwd"`
	StartedAt       time.Time                `json:"startedAt"`
	EndedAt         *time.Time               `json:"endedAt"`
	Usage           conversation.TokenUsage  `json:"usage"`
	Error           *string                  `json:"error"`
	MessageCount    int                      `json:"messageCount"`
	Source          SourceRef                `json:"source"`
}

type ToolRelation struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	RoundID string      `json:"roundId"`
	Calls   []SourceRef `json:"calls"`
	Results []SourceRef `json:"results"`
	Status  string      `json:"status"`
}

type Detail struct {
	Entry           SessionEntry           `json:"entry"`
	Revision        string                 `json:"revision"`
	SessionID       string                 `json:"sessionId"`
	Title           *string                `json:"title"`
	AgentCode       shared.Code            `json:"agentCode"`
	CurrentModel    *shared.ModelRef       `json:"currentModel"`
	Cwd             *string                `json:"cwd"`
	ContextWindow   int64                  `json:"contextWindow"`
	ReasoningEffort shared.ReasoningEffort `json:"reasoningEffort"`
	Rounds          []RoundNode            `json:"rounds"`
	RecordCount     int                    `json:"recordCount"`
	MessageCount    int                    `json:"messageCount"`
	HiddenCount     int                    `json:"hiddenCount"`
	ReplayedThrough int                    `json:"replayedThrough"`
	Complete        bool                   `json:"complete"`
	Diagnostics     []Diagnostic           `json:"diagnostics"`
	Tools           []ToolRelation         `json:"tools"`
}

type RecordDetail struct {
	RawBase64 string           `json:"rawBase64,omitempty"`
	Record    Record           `json:"record"`
	Raw       string           `json:"raw"`
	Envelope  *shared.Envelope `json:"envelope"`
}

type ContextMessage struct {
	Message conversation.Message `json:"message"`
	Kind    string               `json:"kind"`
	Sources []SourceRef          `json:"sources"`
}

type ContextView struct {
	Revision        string           `json:"revision"`
	AtRecord        string           `json:"atRecord"`
	ReplayedThrough int              `json:"replayedThrough"`
	Complete        bool             `json:"complete"`
	Before          []ContextMessage `json:"before"`
	After           []ContextMessage `json:"after"`
}

type Page[T any] struct {
	Items      []T    `json:"items"`
	Total      int    `json:"total"`
	NextOffset *int   `json:"nextOffset"`
	Revision   string `json:"revision"`
}

func Paginate[T any](items []T, offset, limit int, revision string) Page[T] {
	offset = min(offset, len(items))
	end := min(offset+limit, len(items))
	page := Page[T]{Items: items[offset:end], Total: len(items), Revision: revision}
	if end < len(items) {
		page.NextOffset = &end
	}
	return page
}
