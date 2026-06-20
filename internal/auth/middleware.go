package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/timothydodd/ntunl/internal/store"
)

// Authenticator resolves sessions and tokens against the store and provides
// net/http middleware for portal routes.
type Authenticator struct {
	Store *store.Store
}

// NewAuthenticator builds an Authenticator.
func NewAuthenticator(s *store.Store) *Authenticator {
	return &Authenticator{Store: s}
}

type ctxKey int

const userCtxKey ctxKey = 0

// UserFromContext returns the authenticated user attached by RequireAuth, or nil.
func UserFromContext(ctx context.Context) *store.User {
	u, _ := ctx.Value(userCtxKey).(*store.User)
	return u
}

// userFromCookie resolves the session cookie to a live, non-disabled user.
func (a *Authenticator) userFromCookie(r *http.Request) (*store.User, error) {
	c, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, err
	}
	sess, err := a.Store.GetSession(c.Value)
	if err != nil {
		return nil, err
	}
	user, err := a.Store.GetUser(sess.UserID)
	if err != nil {
		return nil, err
	}
	if user.Disabled {
		return nil, errors.New("user disabled")
	}
	return user, nil
}

// RequireAuth wraps a handler, redirecting to /login when there is no valid
// session and attaching the user to the request context otherwise.
func (a *Authenticator) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := a.userFromCookie(r)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userCtxKey, user)))
	}
}

// RequireAdmin is like RequireAuth but additionally requires the admin role.
func (a *Authenticator) RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return a.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		if u := UserFromContext(r.Context()); u == nil || !u.IsAdmin() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	})
}

// ResolveToken maps a plaintext API token to its (live, non-disabled) user and
// stamps last-used. Used by the tunnel handshake and any token-auth API.
func (a *Authenticator) ResolveToken(plaintext string) (*store.User, error) {
	tok, err := a.Store.GetTokenByHash(HashToken(plaintext))
	if err != nil {
		return nil, err
	}
	user, err := a.Store.GetUser(tok.UserID)
	if err != nil {
		return nil, err
	}
	if user.Disabled {
		return nil, errors.New("user disabled")
	}
	_ = a.Store.TouchToken(tok.ID)
	return user, nil
}

// BearerToken extracts a token from an "Authorization: Bearer <token>" header.
func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if strings.HasPrefix(h, prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}
