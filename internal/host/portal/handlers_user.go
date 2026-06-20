package portal

import (
	"net/http"
	"strconv"
	"time"

	"github.com/timothydodd/ntunl/internal/auth"
)

type subdomainRow struct {
	Name        string
	URL         string
	Online      bool
	RemoteAddr  string
	ConnectedAt string
	Requests    int64
}

type requestRow struct {
	Time   string
	Method string
	Path   string
	Status int
}

func (p *Portal) handleDashboard(w http.ResponseWriter, r *http.Request) {
	p.renderDashboard(w, r, "", "")
}

// renderDashboard builds and renders the user dashboard with optional flash text.
func (p *Portal) renderDashboard(w http.ResponseWriter, r *http.Request, errMsg, okMsg string) {
	user := auth.UserFromContext(r.Context())

	rows := make([]subdomainRow, 0)
	for _, lt := range p.live.ActiveByUser(user.ID) {
		rows = append(rows, subdomainRow{
			Name:        lt.Subdomain,
			URL:         publicURL(p.domain, lt.Subdomain),
			Online:      true,
			RemoteAddr:  lt.RemoteAddr,
			ConnectedAt: lt.ConnectedAt.Format("2006-01-02 15:04:05"),
			Requests:    lt.Requests,
		})
	}

	reqRows := make([]requestRow, 0)
	for _, lr := range p.live.RecentRequests(user.ID, 25) {
		reqRows = append(reqRows, requestRow{
			Time:   lr.Time.Format("15:04:05"),
			Method: lr.Method,
			Path:   lr.Path,
			Status: lr.Status,
		})
	}

	p.render(w, "dashboard", map[string]any{
		"Title":      "Dashboard",
		"User":       user,
		"Subdomains": rows,
		"Requests":   reqRows,
		"Error":      errMsg,
		"Ok":         okMsg,
	})
}

func (p *Portal) handleTokens(w http.ResponseWriter, r *http.Request) {
	p.renderTokens(w, r, "", "")
}

func (p *Portal) renderTokens(w http.ResponseWriter, r *http.Request, newToken, errMsg string) {
	user := auth.UserFromContext(r.Context())
	tokens, _ := p.store.ListTokens(user.ID)

	type tokenRow struct {
		ID       int64
		Name     string
		Created  string
		LastUsed string
		Revoked  bool
	}
	rows := make([]tokenRow, 0, len(tokens))
	for _, t := range tokens {
		rows = append(rows, tokenRow{
			ID:       t.ID,
			Name:     t.Name,
			Created:  shortTime(t.CreatedAt),
			LastUsed: shortTime(t.LastUsedAt),
			Revoked:  t.Revoked,
		})
	}

	p.render(w, "tokens", map[string]any{
		"Title":    "API Tokens",
		"User":     user,
		"Tokens":   rows,
		"NewToken": newToken,
		"Error":    errMsg,
	})
}

func (p *Portal) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	name := r.FormValue("name")
	if name == "" {
		name = "token"
	}

	plaintext, hash, err := auth.GenerateToken()
	if err != nil {
		p.renderTokens(w, r, "", "Could not generate token.")
		return
	}
	if _, err := p.store.CreateToken(user.ID, name, hash); err != nil {
		p.renderTokens(w, r, "", "Could not save token.")
		return
	}
	p.renderTokens(w, r, plaintext, "")
}

func (p *Portal) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err == nil {
		_ = p.store.RevokeToken(id, user.ID)
	}
	http.Redirect(w, r, "/tokens", http.StatusSeeOther)
}

func (p *Portal) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	current := r.FormValue("current_password")
	next := r.FormValue("new_password")
	confirm := r.FormValue("confirm_password")

	if !auth.CheckPassword(user.PasswordHash, current) {
		p.renderDashboard(w, r, "Current password is incorrect.", "")
		return
	}
	if len(next) < 8 {
		p.renderDashboard(w, r, "New password must be at least 8 characters.", "")
		return
	}
	if next != confirm {
		p.renderDashboard(w, r, "New passwords do not match.", "")
		return
	}
	hash, err := auth.HashPassword(next)
	if err != nil {
		p.renderDashboard(w, r, "Could not update password.", "")
		return
	}
	if err := p.store.SetUserPassword(user.ID, hash); err != nil {
		p.renderDashboard(w, r, "Could not update password.", "")
		return
	}
	p.renderDashboard(w, r, "", "Password updated.")
}

// shortTime renders a stored RFC3339 timestamp as "2006-01-02 15:04", or "—".
func shortTime(rfc string) string {
	if rfc == "" {
		return "—"
	}
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return rfc
	}
	return t.Local().Format("2006-01-02 15:04")
}
