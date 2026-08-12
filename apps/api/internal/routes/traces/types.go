package traces

import "errors"

var ErrNoTrace = errors.New("no such call")

type Trace struct {
	ID               int64   `json:"id"`
	Purpose          string  `json:"purpose"`
	Model            string  `json:"model"`
	ToolName         string  `json:"toolName"`
	State            string  `json:"state"`
	StartedAt        string  `json:"startedAt"`
	FinishedAt       *string `json:"finishedAt"`
	DurationMs       *int64  `json:"durationMs"`
	InputTokens      int64   `json:"inputTokens"`
	OutputTokens     int64   `json:"outputTokens"`
	CacheReadTokens  int64   `json:"cacheReadTokens"`
	CacheWriteTokens int64   `json:"cacheWriteTokens"`
	Error            *string `json:"error"`
	System           *string `json:"system,omitempty"`
	User             *string `json:"user,omitempty"`
	Output           *string `json:"output,omitempty"`
}
