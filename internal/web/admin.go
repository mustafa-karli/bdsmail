package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mustafakarli/bdsmail/config"
	"github.com/mustafakarli/bdsmail/internal/admin"
	smtprelay "github.com/mustafakarli/bdsmail/internal/smtp"
	"github.com/mustafakarli/bdsmail/internal/tlsutil"
)

type AdminHandlers struct {
	cfg          *config.Config
	relay        *smtprelay.Relay
	certReloader *tlsutil.CertReloader
}

func NewAdminHandlers(cfg *config.Config, relay *smtprelay.Relay, certReloader *tlsutil.CertReloader) *AdminHandlers {
	return &AdminHandlers{cfg: cfg, relay: relay, certReloader: certReloader}
}

type adminPageData struct {
	Domains    []string
	Result     *admin.DomainResult
	Error      string
	AdminAuthed bool
}

// HandleAdminDomains handles the web UI for domain management.
func (ah *AdminHandlers) HandleAdminDomains(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	if ah.cfg.AdminSecret == "" {
		http.Error(w, "Admin interface not configured (set BDS_ADMIN_SECRET)", http.StatusForbidden)
		return
	}

	// Check admin auth via cookie
	if !ah.isAdminAuthed(r) {
		if r.Method == http.MethodPost && r.FormValue("admin_action") == "login" {
			if r.FormValue("admin_secret") == ah.cfg.AdminSecret {
				http.SetCookie(w, &http.Cookie{
					Name:     "bdsmail_admin",
					Value:    ah.cfg.AdminSecret,
					Path:     "/admin/",
					HttpOnly: true,
					Secure:   true,
					SameSite: http.SameSiteLaxMode,
				})
				http.Redirect(w, r, "/admin/domains", http.StatusSeeOther)
				return
			}
			tmpl.render(w, "layout", pageData{Error: "Invalid admin secret"})
			return
		}
		// Show admin login form
		tmpl.render(w, "layout", pageData{})
		return
	}

	data := adminPageData{
		Domains:     ah.cfg.GetDomains(),
		AdminAuthed: true,
	}

	if r.Method == http.MethodPost && r.FormValue("admin_action") == "add_domain" {
		domain := strings.TrimSpace(r.FormValue("domain"))
		result, err := admin.RegisterDomain(ah.cfg, ah.relay, ah.certReloader, domain)
		if err != nil {
			data.Error = err.Error()
		} else {
			data.Result = result
			data.Domains = ah.cfg.GetDomains()
		}
	}

	tmpl.render(w, "layout", pageData{
		Username: "Admin",
		Email:    "admin",
		Message:  data,
	})
}

// HandleAdminAPI handles the JSON API for domain registration (used by CLI).
func (ah *AdminHandlers) HandleAdminAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check bearer token
	auth := r.Header.Get("Authorization")
	if ah.cfg.AdminSecret == "" || auth != "Bearer "+ah.cfg.AdminSecret {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	result, err := admin.RegisterDomain(ah.cfg, ah.relay, ah.certReloader, req.Domain)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (ah *AdminHandlers) isAdminAuthed(r *http.Request) bool {
	cookie, err := r.Cookie("bdsmail_admin")
	if err != nil {
		return false
	}
	return cookie.Value == ah.cfg.AdminSecret
}

// HandleAdminLogout clears the admin session.
func (ah *AdminHandlers) HandleAdminLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "bdsmail_admin",
		Value:    "",
		Path:     "/admin/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	http.Redirect(w, r, "/admin/domains", http.StatusSeeOther)
}
