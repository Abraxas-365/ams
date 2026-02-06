package orchestator

import (
	"github.com/Abraxas-365/ams/pkg/ai/llm"
	"github.com/Abraxas-365/ams/pkg/ai/llm/memoryx"
)

// MemoryFactory creates memory instances for conversations
type MemoryFactory interface {
	Create(systemMessage llm.Message) memoryx.Memory
}

// BufferMemoryFactory creates buffer memory instances
type BufferMemoryFactory struct {
	maxMessages int // 0 = unlimited
}

// NewBufferMemoryFactory creates a new buffer memory factory
func NewBufferMemoryFactory() *BufferMemoryFactory {
	return &BufferMemoryFactory{
		maxMessages: 0, // Unlimited by default
	}
}

// NewBufferMemoryFactoryWithLimit creates a factory with message limit
func NewBufferMemoryFactoryWithLimit(maxMessages int) *BufferMemoryFactory {
	return &BufferMemoryFactory{
		maxMessages: maxMessages,
	}
}

// Create creates a new buffer memory with the given system message
func (f *BufferMemoryFactory) Create(systemMessage llm.Message) memoryx.Memory {
	if f.maxMessages > 0 {
		return memoryx.NewBufferMemory(
			systemMessage,
			memoryx.WithMaxMessages(f.maxMessages),
		)
	}
	return memoryx.NewBufferMemory(systemMessage)
}
