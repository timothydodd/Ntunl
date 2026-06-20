package store

import "errors"

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("not found")

// Role values.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// User is an account row.
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	Disabled     bool
	CreatedAt    string
}

// IsAdmin reports whether the user has the admin role.
func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }

// Token is an API token row. The plaintext value is never stored; only its hash.
type Token struct {
	ID         int64
	UserID     int64
	Name       string
	TokenHash  string
	CreatedAt  string
	LastUsedAt string
	Revoked    bool
}

// Session is a portal browser session.
type Session struct {
	ID        string
	UserID    int64
	CreatedAt string
	ExpiresAt string
}

// TunnelEvent records one connect/disconnect span.
type TunnelEvent struct {
	ID             int64
	UserID         int64
	Subdomain      string
	RemoteAddr     string
	ConnectedAt    string
	DisconnectedAt string
}
