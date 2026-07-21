package conversation

type TokenUsage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CachedRead int64 `json:"cachedRead,omitempty"`
	CacheWrite int64 `json:"cacheWrite,omitempty"`
	Reasoning  int64 `json:"reasoning,omitempty"`
	Total      int64 `json:"total"`
}

func (u TokenUsage) Add(o TokenUsage) TokenUsage {
	return TokenUsage{
		Input:      u.Input + o.Input,
		Output:     u.Output + o.Output,
		CachedRead: u.CachedRead + o.CachedRead,
		CacheWrite: u.CacheWrite + o.CacheWrite,
		Reasoning:  u.Reasoning + o.Reasoning,
		Total:      u.Total + o.Total,
	}
}
