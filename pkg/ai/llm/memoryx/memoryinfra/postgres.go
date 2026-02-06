package memoryinfra

import (
	"context"
	"database/sql"

	"github.com/Abraxas-365/ams/pkg/ai/llm/memoryx"
	"github.com/Abraxas-365/ams/pkg/logx"
	"github.com/jmoiron/sqlx"
)

type PostgresSessionRepository struct {
	db *sqlx.DB
}

func NewPostgresSessionRepository(db *sqlx.DB) memoryx.SessionRepository {
	logx.Info("PostgreSQL session repository initialized")
	return &PostgresSessionRepository{db: db}
}

// CreateSession creates a new session
func (r *PostgresSessionRepository) CreateSession(ctx context.Context, session *memoryx.Session) error {
	executor := r.getExecutor(ctx)

	query := `
        INSERT INTO sessions (id, user_id, title, system_message, created_at, updated_at, is_active)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
    `

	logx.WithFields(logx.Fields{
		"session_id": session.ID,
		"user_id":    session.UserID,
	}).Debug("Creating session")

	_, err := executor.ExecContext(ctx, query,
		session.ID,
		session.UserID,
		session.Title,
		session.SystemMsg,
		session.CreatedAt,
		session.UpdatedAt,
		session.IsActive,
	)

	if err != nil {
		logx.WithError(err).Error("Failed to create session")
		return err
	}

	logx.WithField("session_id", session.ID).Info("Session created successfully")
	return nil
}

// GetSession retrieves a session by ID
func (r *PostgresSessionRepository) GetSession(ctx context.Context, sessionID memoryx.SessionID) (*memoryx.Session, error) {
	executor := r.getExecutor(ctx)

	query := `SELECT * FROM sessions WHERE id = $1`

	logx.WithField("session_id", sessionID).Debug("Fetching session")

	var session memoryx.Session
	err := sqlx.GetContext(ctx, executor, &session, query, sessionID)
	if err == sql.ErrNoRows {
		logx.WithField("session_id", sessionID).Warn("Session not found")
		return nil, memoryx.ErrSessionNotFound()
	}

	if err != nil {
		logx.WithError(err).Error("Failed to get session")
		return nil, err
	}

	return &session, nil
}

// GetSessionWithMessages retrieves session with all messages
func (r *PostgresSessionRepository) GetSessionWithMessages(ctx context.Context, sessionID memoryx.SessionID) (*memoryx.SessionWithMessages, error) {
	executor := r.getExecutor(ctx)

	logx.WithField("session_id", sessionID).Debug("Fetching session with messages")

	// Get session
	var session memoryx.Session
	sessionQuery := `SELECT * FROM sessions WHERE id = $1`
	err := sqlx.GetContext(ctx, executor, &session, sessionQuery, sessionID)
	if err == sql.ErrNoRows {
		return nil, memoryx.ErrSessionNotFound()
	}
	if err != nil {
		logx.WithError(err).Error("Failed to get session")
		return nil, err
	}

	// Get messages
	var messages []memoryx.SessionMessage
	messagesQuery := `SELECT * FROM session_messages WHERE session_id = $1 ORDER BY created_at ASC`
	err = sqlx.SelectContext(ctx, executor, &messages, messagesQuery, sessionID)
	if err != nil {
		logx.WithError(err).Error("Failed to get session messages")
		return nil, err
	}

	logx.WithFields(logx.Fields{
		"session_id":    sessionID,
		"message_count": len(messages),
	}).Debug("Session with messages retrieved successfully")

	return &memoryx.SessionWithMessages{
		Session:  session,
		Messages: messages,
	}, nil
}

// ListUserSessions lists all sessions for a user
func (r *PostgresSessionRepository) ListUserSessions(ctx context.Context, userID string, limit, offset int) ([]*memoryx.Session, error) {
	executor := r.getExecutor(ctx)

	query := `
        SELECT * FROM sessions 
        WHERE user_id = $1 AND is_active = true
        ORDER BY updated_at DESC
        LIMIT $2 OFFSET $3
    `

	logx.WithFields(logx.Fields{
		"user_id": userID,
		"limit":   limit,
		"offset":  offset,
	}).Debug("Listing user sessions")

	var sessions []*memoryx.Session
	err := sqlx.SelectContext(ctx, executor, &sessions, query, userID, limit, offset)

	if err != nil {
		logx.WithError(err).Error("Failed to list user sessions")
		return nil, err
	}

	logx.WithFields(logx.Fields{
		"user_id":       userID,
		"session_count": len(sessions),
	}).Debug("User sessions listed successfully")

	return sessions, nil
}

// UpdateSession updates session metadata
func (r *PostgresSessionRepository) UpdateSession(ctx context.Context, session *memoryx.Session) error {
	executor := r.getExecutor(ctx)

	query := `
        UPDATE sessions 
        SET title = $1, updated_at = $2, is_active = $3
        WHERE id = $4
    `

	logx.WithField("session_id", session.ID).Debug("Updating session")

	_, err := executor.ExecContext(ctx, query,
		session.Title,
		session.UpdatedAt,
		session.IsActive,
		session.ID,
	)

	if err != nil {
		logx.WithError(err).Error("Failed to update session")
		return err
	}

	return nil
}

// DeleteSession soft deletes a session
func (r *PostgresSessionRepository) DeleteSession(ctx context.Context, sessionID memoryx.SessionID) error {
	executor := r.getExecutor(ctx)

	query := `UPDATE sessions SET is_active = false WHERE id = $1`

	logx.WithField("session_id", sessionID).Info("Deleting session")

	_, err := executor.ExecContext(ctx, query, sessionID)

	if err != nil {
		logx.WithError(err).Error("Failed to delete session")
		return err
	}

	logx.WithField("session_id", sessionID).Info("Session deleted successfully")
	return nil
}

// AddMessage adds a message to the session
func (r *PostgresSessionRepository) AddMessage(ctx context.Context, message *memoryx.SessionMessage) error {
	executor := r.getExecutor(ctx)

	query := `
        INSERT INTO session_messages (session_id, role, content, tool_calls, tool_call_id, created_at)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id
    `

	logx.WithFields(logx.Fields{
		"session_id": message.SessionID,
		"role":       message.Role,
	}).Debug("Adding message to session")

	err := executor.QueryRowxContext(ctx, query,
		message.SessionID,
		message.Role,
		message.Content,
		message.ToolCalls,
		message.ToolCallID,
		message.CreatedAt,
	).Scan(&message.ID)

	if err != nil {
		logx.WithError(err).Error("Failed to add message")
		return err
	}

	// Update session's updated_at
	updateQuery := `UPDATE sessions SET updated_at = $1 WHERE id = $2`
	_, _ = executor.ExecContext(ctx, updateQuery, message.CreatedAt, message.SessionID)

	logx.WithField("message_id", message.ID).Debug("Message added successfully")
	return nil
}

// GetMessages retrieves all messages for a session
func (r *PostgresSessionRepository) GetMessages(ctx context.Context, sessionID memoryx.SessionID) ([]memoryx.SessionMessage, error) {
	executor := r.getExecutor(ctx)

	query := `SELECT * FROM session_messages WHERE session_id = $1 ORDER BY created_at ASC`

	var messages []memoryx.SessionMessage
	err := sqlx.SelectContext(ctx, executor, &messages, query, sessionID)

	if err != nil {
		logx.WithError(err).Error("Failed to get messages")
		return nil, err
	}

	return messages, nil
}

// ClearMessages deletes all messages except system message
func (r *PostgresSessionRepository) ClearMessages(ctx context.Context, sessionID memoryx.SessionID) error {
	executor := r.getExecutor(ctx)

	query := `DELETE FROM session_messages WHERE session_id = $1 AND role != 'system'`

	logx.WithField("session_id", sessionID).Debug("Clearing session messages")

	_, err := executor.ExecContext(ctx, query, sessionID)

	if err != nil {
		logx.WithError(err).Error("Failed to clear messages")
		return err
	}

	return nil
}

// GetMessageCount returns the number of messages in a session
func (r *PostgresSessionRepository) GetMessageCount(ctx context.Context, sessionID memoryx.SessionID) (int, error) {
	executor := r.getExecutor(ctx)

	query := `SELECT COUNT(*) FROM session_messages WHERE session_id = $1`

	var count int
	err := executor.QueryRowxContext(ctx, query, sessionID).Scan(&count)

	return count, err
}

// getExecutor returns transaction if in context, otherwise db
func (r *PostgresSessionRepository) getExecutor(ctx context.Context) sqlx.ExtContext {
	if tx, ok := ctx.Value("db_tx").(*sqlx.Tx); ok {
		return tx
	}
	return r.db
}
