package portal

import (
	"net/http"
	"strconv"

	"github.com/timothydodd/ntunl/internal/auth"
	"github.com/timothydodd/ntunl/internal/store"
)

func (p *Portal) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	p.renderAdminUsers(w, r, "", "")
}

func (p *Portal) renderAdminUsers(w http.ResponseWriter, r *http.Request, errMsg, okMsg string) {
	users, _ := p.store.ListUsers()
	p.render(w, "admin_users", map[string]any{
		"Title": "Users",
		"User":  auth.UserFromContext(r.Context()),
		"Users": users,
		"Error": errMsg,
		"Ok":    okMsg,
	})
}

func (p *Portal) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")
	role := r.FormValue("role")
	if role != store.RoleAdmin {
		role = store.RoleUser
	}
	if username == "" || len(password) < 8 {
		p.renderAdminUsers(w, r, "Username required and password must be at least 8 characters.", "")
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		p.renderAdminUsers(w, r, "Could not hash password.", "")
		return
	}
	if _, err := p.store.CreateUser(username, hash, role); err != nil {
		p.renderAdminUsers(w, r, "Could not create user (username taken?).", "")
		return
	}
	p.renderAdminUsers(w, r, "", "Created user "+username+".")
}

func (p *Portal) handleDisableUser(w http.ResponseWriter, r *http.Request) {
	if id, err := strconv.ParseInt(r.PathValue("id"), 10, 64); err == nil {
		_ = p.store.SetUserDisabled(id, true)
		_ = p.store.DeleteUserSessions(id)
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (p *Portal) handleEnableUser(w http.ResponseWriter, r *http.Request) {
	if id, err := strconv.ParseInt(r.PathValue("id"), 10, 64); err == nil {
		_ = p.store.SetUserDisabled(id, false)
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (p *Portal) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	password := r.FormValue("password")
	if err != nil || len(password) < 8 {
		p.renderAdminUsers(w, r, "Password must be at least 8 characters.", "")
		return
	}
	hash, herr := auth.HashPassword(password)
	if herr != nil {
		p.renderAdminUsers(w, r, "Could not hash password.", "")
		return
	}
	_ = p.store.SetUserPassword(id, hash)
	_ = p.store.DeleteUserSessions(id)
	p.renderAdminUsers(w, r, "", "Password reset.")
}

type adminTunnelRow struct {
	Username    string
	Subdomain   string
	RemoteAddr  string
	ConnectedAt string
	Requests    int64
}

func (p *Portal) handleAdminTunnels(w http.ResponseWriter, r *http.Request) {
	users, _ := p.store.ListUsers()
	nameByID := map[int64]string{}
	for _, u := range users {
		nameByID[u.ID] = u.Username
	}

	rows := make([]adminTunnelRow, 0)
	for _, lt := range p.live.AllActive() {
		rows = append(rows, adminTunnelRow{
			Username:    nameByID[lt.UserID],
			Subdomain:   lt.Subdomain,
			RemoteAddr:  lt.RemoteAddr,
			ConnectedAt: lt.ConnectedAt.Format("2006-01-02 15:04:05"),
			Requests:    lt.Requests,
		})
	}

	p.render(w, "admin_tunnels", map[string]any{
		"Title":   "Active Tunnels",
		"User":    auth.UserFromContext(r.Context()),
		"Tunnels": rows,
	})
}
