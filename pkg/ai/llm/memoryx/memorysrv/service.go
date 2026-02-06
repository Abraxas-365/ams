package memorysrv

import (
	"context"
	"time"

	"github.com/Abraxas-365/ams/pkg/ai/llm"
	"github.com/Abraxas-365/ams/pkg/ai/llm/memoryx"
	"github.com/Abraxas-365/ams/pkg/logx"
)

type SessionService struct {
	repository memoryx.SessionRepository
}

func NewSessionService(repo memoryx.SessionRepository) *SessionService {
	logx.Info("Session service initialized")
	return &SessionService{repository: repo}
}

// CreateSession creates a new chat session
func (s *SessionService) CreateSession(ctx context.Context, userID, title string, systemMessage llm.Message) (*memoryx.Session, error) {
	logx.WithFields(logx.Fields{
		"user_id": userID,
		"title":   title,
	}).Info("Creating new session")

	session := &memoryx.Session{
		ID:        memoryx.NewSessionID(),
		UserID:    userID,
		Title:     title,
		SystemMsg: systemMessage.Content,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		IsActive:  true,
	}

	if err := s.repository.CreateSession(ctx, session); err != nil {
		logx.WithError(err).Error("Failed to create session")
		return nil, err
	}

	// Add system message
	sysMsg, err := memoryx.FromLLMMessage(session.ID, systemMessage)
	if err != nil {
		logx.WithError(err).Error("Failed to convert system message")
		return nil, err
	}

	if err := s.repository.AddMessage(ctx, &sysMsg); err != nil {
		logx.WithError(err).Error("Failed to add system message")
		return nil, err
	}

	logx.WithField("session_id", session.ID).Info("Session created successfully")
	return session, nil
}

// GetSessionMemory creates a Memory instance for a session
func (s *SessionService) GetSessionMemory(ctx context.Context, sessionID memoryx.SessionID) (memoryx.Memory, error) {
	logx.WithField("session_id", sessionID).Debug("Getting session memory")

	// Verify session exists
	session, err := s.repository.GetSession(ctx, sessionID)
	if err != nil {
		logx.WithError(err).Error("Failed to get session")
		return nil, err
	}

	if !session.IsActive {
		logx.WithField("session_id", sessionID).Warn("Attempting to use inactive session")
		return nil, memoryx.ErrSessionInactive()
	}

	return memoryx.NewSessionMemory(ctx, sessionID, s.repository), nil
}

// ListUserSessions lists all sessions for a user
func (s *SessionService) ListUserSessions(ctx context.Context, userID string, limit, offset int) ([]*memoryx.Session, error) {
	logx.WithFields(logx.Fields{
		"user_id": userID,
		"limit":   limit,
		"offset":  offset,
	}).Debug("Listing user sessions")

	return s.repository.ListUserSessions(ctx, userID, limit, offset)
}

// GetSession retrieves a session by ID
func (s *SessionService) GetSession(ctx context.Context, sessionID memoryx.SessionID) (*memoryx.Session, error) {
	return s.repository.GetSession(ctx, sessionID)
}

// GetSessionWithMessages retrieves session with messages
func (s *SessionService) GetSessionWithMessages(ctx context.Context, sessionID memoryx.SessionID) (*memoryx.SessionWithMessages, error) {
	return s.repository.GetSessionWithMessages(ctx, sessionID)
}

// UpdateSessionTitle updates the session title
func (s *SessionService) UpdateSessionTitle(ctx context.Context, sessionID memoryx.SessionID, title string) error {
	session, err := s.repository.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.Title = title
	session.UpdatedAt = time.Now()

	return s.repository.UpdateSession(ctx, session)
}

// DeleteSession soft deletes a session
func (s *SessionService) DeleteSession(ctx context.Context, sessionID memoryx.SessionID) error {
	logx.WithField("session_id", sessionID).Info("Deleting session")
	return s.repository.DeleteSession(ctx, sessionID)
}

// ClearSessionMessages clears all messages except system message
func (s *SessionService) ClearSessionMessages(ctx context.Context, sessionID memoryx.SessionID) error {
	logx.WithField("session_id", sessionID).Info("Clearing session messages")
	return s.repository.ClearMessages(ctx, sessionID)
}
