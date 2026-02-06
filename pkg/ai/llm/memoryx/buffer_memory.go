package memoryx

import (
	"sync"

	"github.com/Abraxas-365/ams/pkg/ai/llm"
)

// BufferMemory stores conversation messages in memory
type BufferMemory struct {
	mu            sync.RWMutex
	systemMessage llm.Message
	messages      []llm.Message
	maxMessages   int // 0 = unlimited
}

// BufferMemoryOption configures buffer memory
type BufferMemoryOption func(*BufferMemory)

// WithMaxMessages sets the maximum number of messages to retain
func WithMaxMessages(max int) BufferMemoryOption {
	return func(m *BufferMemory) {
		m.maxMessages = max
	}
}

// NewBufferMemory creates a new buffer memory with a system message
func NewBufferMemory(systemMessage llm.Message, opts ...BufferMemoryOption) *BufferMemory {
	memory := &BufferMemory{
		systemMessage: systemMessage,
		messages:      make([]llm.Message, 0),
		maxMessages:   0, // Unlimited by default
	}

	for _, opt := range opts {
		opt(memory)
	}

	return memory
}

// Messages returns all messages including the system message
func (m *BufferMemory) Messages() ([]llm.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Always include system message first
	allMessages := make([]llm.Message, 0, len(m.messages)+1)

	// Add system message if it has content
	if m.systemMessage.Content != "" {
		allMessages = append(allMessages, m.systemMessage)
	}

	// Add conversation messages
	allMessages = append(allMessages, m.messages...)

	return allMessages, nil
}

// Add adds a new message to memory
func (m *BufferMemory) Add(message llm.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.messages = append(m.messages, message)

	// Trim messages if max limit is set and exceeded
	if m.maxMessages > 0 && len(m.messages) > m.maxMessages {
		// Keep the most recent messages
		trimCount := len(m.messages) - m.maxMessages
		m.messages = m.messages[trimCount:]
	}

	return nil
}

// Clear removes all conversation messages but keeps the system message
func (m *BufferMemory) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.messages = make([]llm.Message, 0)

	return nil
}

// GetSystemMessage returns the system message
func (m *BufferMemory) GetSystemMessage() llm.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.systemMessage
}

// SetSystemMessage updates the system message
func (m *BufferMemory) SetSystemMessage(message llm.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.systemMessage = message
}

// Count returns the number of messages (excluding system message)
func (m *BufferMemory) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.messages)
}

// GetLastN returns the last N messages (excluding system message)
func (m *BufferMemory) GetLastN(n int) []llm.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if n <= 0 || len(m.messages) == 0 {
		return []llm.Message{}
	}

	if n >= len(m.messages) {
		// Return a copy of all messages
		result := make([]llm.Message, len(m.messages))
		copy(result, m.messages)
		return result
	}

	// Return the last N messages
	start := len(m.messages) - n
	result := make([]llm.Message, n)
	copy(result, m.messages[start:])
	return result
}

// GetByRole returns all messages with a specific role
func (m *BufferMemory) GetByRole(role string) []llm.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]llm.Message, 0)
	for _, msg := range m.messages {
		if msg.Role == role {
			result = append(result, msg)
		}
	}

	return result
}

// Replace replaces all messages (excluding system message)
func (m *BufferMemory) Replace(messages []llm.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.messages = make([]llm.Message, len(messages))
	copy(m.messages, messages)

	return nil
}
