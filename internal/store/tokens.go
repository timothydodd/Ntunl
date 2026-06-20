package store

import (
	"database/sql"
	"errors"
)

// CreateToken stores a token (by its hash) for a user and returns its id.
func (s *Store) CreateToken(userID int64, name, tokenHash string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO tokens (user_id, name, token_hash, created_at) VALUES (?, ?, ?, ?)`,
		userID, name, tokenHash, now(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetTokenByHash returns a non-revoked token matching the hash.
func (s *Store) GetTokenByHash(tokenHash string) (*Token, error) {
	var t Token
	var lastUsed sql.NullString
	err := s.db.QueryRow(
		`SELECT id, user_id, name, token_hash, created_at, last_used_at, revoked
		   FROM tokens WHERE token_hash = ? AND revoked = 0`,
		tokenHash,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.CreatedAt, &lastUsed, &t.Revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.LastUsedAt = lastUsed.String
	return &t, nil
}

// ListTokens returns a user's tokens (including revoked) newest first.
func (s *Store) ListTokens(userID int64) ([]Token, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, name, token_hash, created_at, last_used_at, revoked
		   FROM tokens WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []Token
	for rows.Next() {
		var t Token
		var lastUsed sql.NullString
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.CreatedAt, &lastUsed, &t.Revoked); err != nil {
			return nil, err
		}
		t.LastUsedAt = lastUsed.String
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// TouchToken records that a token was just used.
func (s *Store) TouchToken(id int64) error {
	_, err := s.db.Exec(`UPDATE tokens SET last_used_at = ? WHERE id = ?`, now(), id)
	return err
}

// RevokeToken marks a token revoked. It is scoped to userID so users can only
// revoke their own tokens.
func (s *Store) RevokeToken(id, userID int64) error {
	_, err := s.db.Exec(`UPDATE tokens SET revoked = 1 WHERE id = ? AND user_id = ?`, id, userID)
	return err
}
