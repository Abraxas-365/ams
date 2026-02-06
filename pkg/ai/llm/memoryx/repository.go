package memoryx

import "context"

// SessionRepository manages session persistence
type SessionRepository interface {
	// Session CRUD
	CreateSession(ctx context.Context, session *Session) error
	GetSession(ctx context.Context, sessionID SessionID) (*Session, error)
	GetSessionWithMessages(ctx context.Context, sessionID SessionID) (*SessionWithMessages, error)
	ListUserSessions(ctx context.Context, userID string, limit, offset int) ([]*Session, error)
	UpdateSession(ctx context.Context, session *Session) error
	DeleteSession(ctx context.Context, sessionID SessionID) error

	// Message operations
	AddMessage(ctx context.Context, message *SessionMessage) error
	GetMessages(ctx context.Context, sessionID SessionID) ([]SessionMessage, error)
	ClearMessages(ctx context.Context, sessionID SessionID) error
	GetMessageCount(ctx context.Context, sessionID SessionID) (int, error)
}
