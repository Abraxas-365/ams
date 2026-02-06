// pkg/ai/llm/memoryx/session_memory.go
package memoryx

import (
	"context"
	"github.com/Abraxas-365/ams/pkg/ai/llm"
	"github.com/Abraxas-365/ams/pkg/logx"
)

// SessionMemory implements Memory interface with database persistence
type SessionMemory struct {
	sessionID  SessionID
	repository SessionRepository
	ctx        context.Context
}

// NewSessionMemory creates memory bound to a session
func NewSessionMemory(ctx context.Context, sessionID SessionID, repo SessionRepository) Memory {
	logx.WithField("session_id", sessionID).Debug("Creating session memory")
	return &SessionMemory{
		sessionID:  sessionID,
		repository: repo,
		ctx:        ctx,
	}
}

// Messages returns all messages including system prompt
func (m *SessionMemory) Messages() ([]llm.Message, error) {
	logx.WithField("session_id", m.sessionID).Debug("Retrieving messages from session")

	sessionMessages, err := m.repository.GetMessages(m.ctx, m.sessionID)
	if err != nil {
		logx.WithError(err).Error("Failed to get messages from repository")
		return nil, err
	}

	messages := make([]llm.Message, 0, len(sessionMessages))
	for _, sm := range sessionMessages {
		msg, err := sm.ToLLMMessage()
		if err != nil {
			logx.WithError(err).Warn("Failed to convert session message to LLM message")
			continue
		}
		messages = append(messages, msg)
	}

	logx.WithFields(logx.Fields{
		"session_id":    m.sessionID,
		"message_count": len(messages),
	}).Debug("Messages retrieved successfully")

	return messages, nil
}

// Add adds a new message to the session
func (m *SessionMemory) Add(message llm.Message) error {
	logx.WithFields(logx.Fields{
		"session_id": m.sessionID,
		"role":       message.Role,
	}).Debug("Adding message to session")

	sessionMsg, err := FromLLMMessage(m.sessionID, message)
	if err != nil {
		logx.WithError(err).Error("Failed to convert LLM message to session message")
		return err
	}

	if err := m.repository.AddMessage(m.ctx, &sessionMsg); err != nil {
		logx.WithError(err).Error("Failed to add message to repository")
		return err
	}

	logx.WithField("session_id", m.sessionID).Debug("Message added successfully")
	return nil
}

// Clear removes all messages except system message
func (m *SessionMemory) Clear() error {
	logx.WithField("session_id", m.sessionID).Info("Clearing session messages")

	err := m.repository.ClearMessages(m.ctx, m.sessionID)
	if err != nil {
		logx.WithError(err).Error("Failed to clear messages")
		return err
	}

	logx.WithField("session_id", m.sessionID).Debug("Session messages cleared successfully")
	return nil
}

// GetSessionID returns the current session ID
func (m *SessionMemory) GetSessionID() SessionID {
	return m.sessionID
}
