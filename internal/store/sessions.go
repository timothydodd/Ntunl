package store

import (
	"database/sql"
	"errors"
	"time"
)

// CreateSession stores a session row.
func (s *Store) CreateSession(id string, userID int64, ttl time.Duration) error {
	created := time.Now().UTC()
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		id, userID, created.Format(time.RFC3339), created.Add(ttl).Format(time.RFC3339),
	)
	return err
}

// GetSession returns a session by id if it exists and has not expired.
func (s *Store) GetSession(id string) (*Session, error) {
	var sess Session
	err := s.db.QueryRow(
		`SELECT id, user_id, created_at, expires_at FROM sessions WHERE id = ?`, id,
	).Scan(&sess.ID, &sess.UserID, &sess.CreatedAt, &sess.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	exp, err := time.Parse(time.RFC3339, sess.ExpiresAt)
	if err == nil && time.Now().After(exp) {
		_ = s.DeleteSession(id)
		return nil, ErrNotFound
	}
	return &sess, nil
}

// DeleteSession removes a session (logout).
func (s *Store) DeleteSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// DeleteUserSessions removes all sessions for a user (e.g. on disable).
func (s *Store) DeleteUserSessions(userID int64) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}
