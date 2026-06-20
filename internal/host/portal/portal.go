package portal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/timothydodd/ntunl/internal/auth"
	"github.com/timothydodd/ntunl/internal/store"
)

// Portal is the admin/user web portal and auth API.
type Portal struct {
	log    *slog.Logger
	store  *store.Store
	auth   *auth.Authenticator
	live   LiveTunnels
	domain string
	port   int
	secure bool
}

// Options configures the portal.
type Options struct {
	Log    *slog.Logger
	Store  *store.Store
	Auth   *auth.Authenticator
	Live   LiveTunnels
	Domain string // base domain used to render public URLs
	Port   int
	Secure bool // set Secure flag on the session cookie (behind TLS)
}

// New builds a Portal.
func New(o Options) *Portal {
	return &Portal{
		log:    o.Log,
		store:  o.Store,
		auth:   o.Auth,
		live:   o.Live,
		domain: o.Domain,
		port:   o.Port,
		secure: o.Secure,
	}
}

// Run starts the portal HTTP server and blocks until ctx is cancelled.
func (p *Portal) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", p.port),
		Handler: p.routes(),
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	p.log.Info("Portal is running", "addr", srv.Addr)
	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (p *Portal) routes() http.Handler {
	mux := http.NewServeMux()
	a := p.auth

	// Public auth.
	mux.HandleFunc("GET /login", p.handleLoginForm)
	mux.HandleFunc("POST /login", p.handleLoginSubmit)
	mux.HandleFunc("GET /logout", p.handleLogout)
	mux.HandleFunc("POST /api/auth/login", p.handleAPILogin)

	// User.
	mux.HandleFunc("GET /{$}", a.RequireAuth(p.handleDashboard))
	mux.HandleFunc("GET /tokens", a.RequireAuth(p.handleTokens))
	mux.HandleFunc("POST /tokens", a.RequireAuth(p.handleCreateToken))
	mux.HandleFunc("POST /tokens/{id}/revoke", a.RequireAuth(p.handleRevokeToken))
	mux.HandleFunc("POST /account/password", a.RequireAuth(p.handleChangePassword))

	// Admin.
	mux.HandleFunc("GET /admin/users", a.RequireAdmin(p.handleAdminUsers))
	mux.HandleFunc("POST /admin/users", a.RequireAdmin(p.handleCreateUser))
	mux.HandleFunc("POST /admin/users/{id}/disable", a.RequireAdmin(p.handleDisableUser))
	mux.HandleFunc("POST /admin/users/{id}/enable", a.RequireAdmin(p.handleEnableUser))
	mux.HandleFunc("POST /admin/users/{id}/reset-password", a.RequireAdmin(p.handleResetPassword))
	mux.HandleFunc("GET /admin/tunnels", a.RequireAdmin(p.handleAdminTunnels))

	return mux
}

// setSessionCookie issues a session for the user and sets the cookie.
func (p *Portal) setSessionCookie(w http.ResponseWriter, userID int64) error {
	sid, err := auth.NewSessionID()
	if err != nil {
		return err
	}
	if err := p.store.CreateSession(sid, userID, auth.SessionTTL); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		Secure:   p.secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(auth.SessionTTL),
	})
	return nil
}
