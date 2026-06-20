package store

import (
	"database/sql"
	"errors"
)

// CreateUser inserts a new user and returns its id.
func (s *Store) CreateUser(username, passwordHash, role string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO users (username, password_hash, role, created_at) VALUES (?, ?, ?, ?)`,
		username, passwordHash, role, now(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CountUsers returns the number of user rows.
func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// GetUserByUsername looks up a user by username.
func (s *Store) GetUserByUsername(username string) (*User, error) {
	return s.scanUser(s.db.QueryRow(
		`SELECT id, username, password_hash, role, disabled, created_at FROM users WHERE username = ?`,
		username,
	))
}

// GetUser looks up a user by id.
func (s *Store) GetUser(id int64) (*User, error) {
	return s.scanUser(s.db.QueryRow(
		`SELECT id, username, password_hash, role, disabled, created_at FROM users WHERE id = ?`,
		id,
	))
}

// ListUsers returns all users ordered by username.
func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(
		`SELECT id, username, password_hash, role, disabled, created_at FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Disabled, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// SetUserDisabled enables or disables a user.
func (s *Store) SetUserDisabled(id int64, disabled bool) error {
	_, err := s.db.Exec(`UPDATE users SET disabled = ? WHERE id = ?`, boolToInt(disabled), id)
	return err
}

// SetUserPassword updates a user's password hash.
func (s *Store) SetUserPassword(id int64, passwordHash string) error {
	_, err := s.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, id)
	return err
}

func (s *Store) scanUser(row *sql.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Disabled, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
