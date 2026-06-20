package portal

import (
	"encoding/json"
	"net/http"

	"github.com/timothydodd/ntunl/internal/auth"
)

func (p *Portal) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	p.render(w, "login", map[string]any{"Title": "Sign in"})
}

func (p *Portal) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := p.store.GetUserByUsername(username)
	if err != nil || user.Disabled || !auth.CheckPassword(user.PasswordHash, password) {
		p.render(w, "login", map[string]any{"Title": "Sign in", "Error": "Invalid username or password."})
		return
	}

	if err := p.setSessionCookie(w, user.ID); err != nil {
		p.log.Error("create session", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (p *Portal) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookieName); err == nil {
		_ = p.store.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: auth.SessionCookieName, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// handleAPILogin authenticates username/password and mints an API token for the
// CLI client. Used by `client login`.
func (p *Portal) handleAPILogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}

	user, err := p.store.GetUserByUsername(req.Username)
	if err != nil || user.Disabled || !auth.CheckPassword(user.PasswordHash, req.Password) {
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	plaintext, hash, err := auth.GenerateToken()
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	name := req.Name
	if name == "" {
		name = "cli"
	}
	if _, err := p.store.CreateToken(user.ID, name, hash); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token":    plaintext,
		"username": user.Username,
		"role":     user.Role,
	})
}
