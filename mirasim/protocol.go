package mirasim

import "encoding/json"

// Frame represents a generic Mirasim WebSocket frame.
type Frame struct {
	Type       string          `json:"type"`
	ClientRef  string          `json:"clientRef,omitempty"`
	SessionKey string          `json:"sessionKey,omitempty"`
	Message    string          `json:"message,omitempty"`
	Code       string          `json:"code,omitempty"`
	Seq        int             `json:"seq,omitempty"`
	Patch      *Patch          `json:"patch,omitempty"`
	Snapshot   *Snapshot       `json:"snapshot,omitempty"`
	Agents     []AgentInfo     `json:"agents,omitempty"`
	Raw        json.RawMessage `json:"-"`
}

// ClientPromptFrame is sent by client to start a turn.
type ClientPromptFrame struct {
	Type        string   `json:"type"`
	ClientRef   string   `json:"clientRef"`
	SessionKey  string   `json:"sessionKey,omitempty"`
	Agent       string   `json:"agent,omitempty"`
	Model       string   `json:"model,omitempty"`
	Effort      string   `json:"effort,omitempty"`
	Prompt      string   `json:"prompt"`
	Workdir     string   `json:"workdir,omitempty"`
	Attachments []string `json:"attachments,omitempty"`
}

// ClientSubscribeFrame is sent to subscribe to a session's updates.
type ClientSubscribeFrame struct {
	Type       string `json:"type"`
	SessionKey string `json:"sessionKey"`
}

// ClientStopFrame is sent to cancel a running turn.
type ClientStopFrame struct {
	Type       string `json:"type"`
	SessionKey string `json:"sessionKey"`
}

// AgentInfo describes an agent supported by Mirasim.
type AgentInfo struct {
	ID           string                 `json:"id"`
	Label        string                 `json:"label"`
	Installed    bool                   `json:"installed"`
	Capabilities map[string]interface{} `json:"capabilities,omitempty"`
}

// Patch is the delta update in a "session" frame.
type Patch struct {
	Full            *Snapshot      `json:"full,omitempty"`
	AppendText      string         `json:"appendText,omitempty"`
	AppendReasoning string         `json:"appendReasoning,omitempty"`
	Set             *SnapshotDelta `json:"set,omitempty"`
}

// Snapshot is the full state of a session.
type Snapshot struct {
	Phase      string      `json:"phase"`
	TaskId     string      `json:"taskId"`
	SessionId  *string     `json:"sessionId"`
	Agent      string      `json:"agent"`
	Model      *string     `json:"model"`
	ModelLabel *string     `json:"modelLabel"`
	Effort     string      `json:"effort"`
	Prompt     string      `json:"prompt"`
	Text       string      `json:"text"`
	Reasoning  string      `json:"reasoning,omitempty"`
	Error      interface{} `json:"error"`
	Incomplete bool        `json:"incomplete"`
	UpdatedAt  int64       `json:"updatedAt"`
	Usage      *Usage      `json:"usage,omitempty"`
}

// SnapshotDelta holds updated fields in patch.set.
type SnapshotDelta struct {
	Phase      string      `json:"phase,omitempty"`
	TaskId     string      `json:"taskId,omitempty"`
	SessionId  *string     `json:"sessionId,omitempty"`
	Agent      string      `json:"agent,omitempty"`
	Model      *string     `json:"model,omitempty"`
	ModelLabel *string     `json:"modelLabel,omitempty"`
	Effort     string      `json:"effort,omitempty"`
	Prompt     string      `json:"prompt,omitempty"`
	Text       string      `json:"text,omitempty"`
	Reasoning  string      `json:"reasoning,omitempty"`
	Error      interface{} `json:"error,omitempty"`
	Incomplete bool        `json:"incomplete,omitempty"`
	UpdatedAt  int64       `json:"updatedAt,omitempty"`
	Usage      *Usage      `json:"usage,omitempty"`
}

// Usage holds token consumption statistics.
type Usage struct {
	InputTokens       int `json:"inputTokens"`
	OutputTokens      int `json:"outputTokens"`
	CachedInputTokens int `json:"cachedInputTokens,omitempty"`
	ContextPercent    int `json:"contextPercent,omitempty"`
}
