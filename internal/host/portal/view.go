package portal

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
)

//go:embed templates/*.html
var templatesFS embed.FS

// pages maps a logical page name to a parsed template (layout + page).
var pages map[string]*template.Template

func init() {
	pageFiles := []string{
		"login", "dashboard", "tokens",
		"admin_users", "admin_tunnels",
	}
	pages = make(map[string]*template.Template, len(pageFiles))
	for _, name := range pageFiles {
		t := template.Must(template.ParseFS(templatesFS, "templates/layout.html", "templates/"+name+".html"))
		pages[name] = t
	}
}

// render executes the named page's layout with data.
func (p *Portal) render(w http.ResponseWriter, name string, data any) {
	t, ok := pages[name]
	if !ok {
		http.Error(w, "unknown page: "+name, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		p.log.Error("render page", "page", name, "err", err)
	}
}

// publicURL builds the user-facing URL for a subdomain given the configured base
// domain. Falls back to a bare subdomain when no domain is configured.
func publicURL(domain, subdomain string) string {
	if domain == "" {
		return subdomain
	}
	return fmt.Sprintf("https://%s.%s", subdomain, domain)
}
