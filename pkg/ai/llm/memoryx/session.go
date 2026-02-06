package memoryx

import (
	"encoding/json"
	"math/rand"
	"time"

	"github.com/Abraxas-365/ams/pkg/ai/llm"
)

// SessionID is a unique identifier for a session
type SessionID string

// Session represents a conversation session
type Session struct {
	ID        SessionID `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`
	Title     string    `json:"title" db:"title"`
	SystemMsg string    `json:"system_message" db:"system_message"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
	IsActive  bool      `json:"is_active" db:"is_active"`
}

// SessionMessage represents a message in a session
type SessionMessage struct {
	ID         int64     `json:"id" db:"id"`
	SessionID  SessionID `json:"session_id" db:"session_id"`
	Role       string    `json:"role" db:"role"`
	Content    string    `json:"content" db:"content"`
	ToolCalls  string    `json:"tool_calls,omitempty" db:"tool_calls"` // JSON serialized
	ToolCallID string    `json:"tool_call_id,omitempty" db:"tool_call_id"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

// SessionWithMessages combines session with its messages
type SessionWithMessages struct {
	Session  Session          `json:"session"`
	Messages []SessionMessage `json:"messages"`
}

// NewSessionID generates a new session ID
func NewSessionID() SessionID {
	return SessionID(time.Now().Format("20060102150405") + "-" + randString(8))
}

// ToLLMMessage converts SessionMessage to llm.Message
func (sm *SessionMessage) ToLLMMessage() (llm.Message, error) {
	msg := llm.Message{
		Role:       sm.Role,
		Content:    sm.Content,
		ToolCallID: sm.ToolCallID,
	}

	// Deserialize tool calls if present
	if sm.ToolCalls != "" {
		var toolCalls []llm.ToolCall
		if err := json.Unmarshal([]byte(sm.ToolCalls), &toolCalls); err != nil {
			return msg, err
		}
		msg.ToolCalls = toolCalls
	}

	return msg, nil
}

// FromLLMMessage creates SessionMessage from llm.Message
func FromLLMMessage(sessionID SessionID, msg llm.Message) (SessionMessage, error) {
	sm := SessionMessage{
		SessionID:  sessionID,
		Role:       msg.Role,
		Content:    msg.Content,
		ToolCallID: msg.ToolCallID,
		CreatedAt:  time.Now(),
	}

	// Serialize tool calls if present
	if len(msg.ToolCalls) > 0 {
		toolCallsJSON, err := json.Marshal(msg.ToolCalls)
		if err != nil {
			return sm, err
		}
		sm.ToolCalls = string(toolCallsJSON)
	}

	return sm, nil
}

func randString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
