package store

import (
	"path/filepath"
	"testing"
	"time"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestUserLifecycle(t *testing.T) {
	s := openTemp(t)

	n, err := s.CountUsers()
	if err != nil || n != 0 {
		t.Fatalf("CountUsers=%d err=%v, want 0", n, err)
	}

	id, err := s.CreateUser("alice", "hash", RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	u, err := s.GetUserByUsername("alice")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if u.ID != id || !u.IsAdmin() || u.Disabled {
		t.Fatalf("unexpected user: %+v", u)
	}

	if _, err := s.GetUserByUsername("nobody"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	if err := s.SetUserDisabled(id, true); err != nil {
		t.Fatalf("disable: %v", err)
	}
	u, _ = s.GetUser(id)
	if !u.Disabled {
		t.Fatalf("user should be disabled")
	}
}

func TestTokenLifecycle(t *testing.T) {
	s := openTemp(t)
	uid, _ := s.CreateUser("bob", "hash", RoleUser)

	if _, err := s.CreateToken(uid, "laptop", "deadbeef"); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	tok, err := s.GetTokenByHash("deadbeef")
	if err != nil || tok.UserID != uid {
		t.Fatalf("GetTokenByHash: %+v err=%v", tok, err)
	}
	if err := s.RevokeToken(tok.ID, uid); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := s.GetTokenByHash("deadbeef"); err != ErrNotFound {
		t.Fatalf("revoked token should be ErrNotFound, got %v", err)
	}
}

func TestSessionExpiry(t *testing.T) {
	s := openTemp(t)
	uid, _ := s.CreateUser("carol", "hash", RoleUser)

	if err := s.CreateSession("sess1", uid, time.Hour); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := s.GetSession("sess1"); err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	if err := s.CreateSession("sess2", uid, -time.Hour); err != nil {
		t.Fatalf("CreateSession expired: %v", err)
	}
	if _, err := s.GetSession("sess2"); err != ErrNotFound {
		t.Fatalf("expired session should be ErrNotFound, got %v", err)
	}
}
