// orchestator/dto.go
package orchestator

import "github.com/Abraxas-365/ams/context"

// ChatRequest represents an incoming chat request from the frontend
type ChatRequest struct {
	Message        string                   `json:"message"`
	Route          RouteInfo                `json:"route"`
	Frontend       *context.FrontendContext `json:"frontend,omitempty"`
	User           *context.User            `json:"user,omitempty"`
	ConversationID string                   `json:"conversation_id,omitempty"`
	SessionID      string                   `json:"session_id,omitempty"` // ✅ For session-based memory
	StreamResponse bool                     `json:"stream_response,omitempty"`

	// 🔐 Authentication (user's token from frontend)
	BearerToken   string            `json:"bearer_token,omitempty"`
	CustomHeaders map[string]string `json:"custom_headers,omitempty"`

	// ✅ NEW: Dynamic route context injection
	RouteParams        map[string]string `json:"route_params,omitempty"` // Frontend sends route params to trigger fresh context fetch
	ShouldFetchContext bool              `json:"should_fetch_context"`   // Explicit flag to request fresh backend data
}

// RouteInfo contains information about the current route
type RouteInfo struct {
	Path  string            `json:"path"`
	Query map[string]string `json:"query,omitempty"`
}

// ChatResponse represents the response
type ChatResponse struct {
	Response       string         `json:"response"`
	SessionID      string         `json:"session_id,omitempty"` // ✅ Return session ID
	ConversationID string         `json:"conversation_id,omitempty"`
	Usage          *UsageInfo     `json:"usage,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// UsageInfo contains token usage information
type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StreamChunk represents a chunk in streaming response
type StreamChunk struct {
	Content   string         `json:"content"`
	Done      bool           `json:"done"`
	SessionID string         `json:"session_id,omitempty"` // ✅ For streaming
	Error     string         `json:"error,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}
