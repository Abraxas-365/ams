package memoryx

import "github.com/Abraxas-365/ams/pkg/ai/llm"

// Memory represents a conversation memory with system prompt management
type Memory interface {
	// Messages returns all messages including system prompt
	// May return error if retrieval fails (e.g., database error)
	Messages() ([]llm.Message, error)

	// Add adds a new message to memory
	// Returns error if the operation fails
	Add(message llm.Message) error

	// Clear resets the conversation but keeps the system prompt
	// Returns error if the operation fails
	Clear() error
}
