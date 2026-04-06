package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mustafakarli/bdsmail/config"
	"github.com/mustafakarli/bdsmail/internal/admin"
	"github.com/mustafakarli/bdsmail/internal/model"
	smtprelay "github.com/mustafakarli/bdsmail/internal/smtp"
	"github.com/mustafakarli/bdsmail/internal/store"
	"github.com/mustafakarli/bdsmail/internal/tlsutil"
)

type AdminHandlers struct {
	cfg          *config.Config
	store        *store.Store
	relay        *smtprelay.Relay
	certReloader *tlsutil.CertReloader
}

func NewAdminHandlers(cfg *config.Config, s *store.Store, relay *smtprelay.Relay, certReloader *tlsutil.CertReloader) *AdminHandlers {
	return &AdminHandlers{cfg: cfg, store: s, relay: relay, certReloader: certReloader}
}

type adminPageData struct {
	Domains     []string
	Result      *admin.DomainResult
	Error       string
	Success     string
	AdminAuthed bool
	Users       interface{}
	Aliases     interface{}
	Lists       interface{}
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
		AdminData: data,
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

// HandleAdminUsers handles user provisioning (list, create, delete).
func (ah *AdminHandlers) HandleAdminUsers(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	if !ah.handleAdminAuth(w, r, tmpl, "/admin/users") {
		return
	}

	data := adminPageData{AdminAuthed: true}

	if r.Method == http.MethodPost {
		switch r.FormValue("admin_action") {
		case "create_user":
			email := strings.TrimSpace(r.FormValue("email"))
			displayName := strings.TrimSpace(r.FormValue("display_name"))
			password := r.FormValue("password")
			username, domain := store.SplitEmail(email)
			if username == "" || domain == "" {
				data.Error = "Invalid email format"
			} else {
				hash, err := model.HashPassword(password)
				if err != nil {
					data.Error = "Failed to hash password"
				} else if err := ah.store.DB.CreateUser(username, domain, displayName, hash); err != nil {
					data.Error = "Failed to create user: " + err.Error()
				} else {
					data.Success = "User " + email + " created"
				}
			}
		case "delete_user":
			email := r.FormValue("email")
			ah.store.DB.DeleteUserMessages(email)
			if err := ah.store.DB.DeleteUser(email); err != nil {
				data.Error = "Failed to delete user"
			} else {
				data.Success = "User " + email + " deleted"
			}
		}
	}

	users, _ := ah.store.DB.ListUsers()
	data.Users = users

	tmpl.render(w, "layout", pageData{Username: "Admin", Email: "admin", AdminData: data})
}

// HandleAdminAliases handles alias management.
func (ah *AdminHandlers) HandleAdminAliases(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	if !ah.handleAdminAuth(w, r, tmpl, "/admin/aliases") {
		return
	}

	data := adminPageData{AdminAuthed: true}

	if r.Method == http.MethodPost {
		switch r.FormValue("admin_action") {
		case "create_alias":
			aliasEmail := strings.TrimSpace(r.FormValue("alias_email"))
			targetsStr := r.FormValue("target_emails")
			var targets []string
			for _, t := range strings.Split(targetsStr, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					targets = append(targets, t)
				}
			}
			if err := ah.store.DB.CreateAlias(aliasEmail, targets); err != nil {
				data.Error = "Failed to create alias: " + err.Error()
			} else {
				data.Success = "Alias created"
			}
		case "delete_alias":
			if err := ah.store.DB.DeleteAlias(r.FormValue("alias_email")); err != nil {
				data.Error = "Failed to delete alias"
			} else {
				data.Success = "Alias deleted"
			}
		}
	}

	aliases, _ := ah.store.DB.ListAliases()
	data.Aliases = aliases

	tmpl.render(w, "layout", pageData{Username: "Admin", Email: "admin", AdminData: data})
}

// HandleAdminLists handles mailing list management.
func (ah *AdminHandlers) HandleAdminLists(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	if !ah.handleAdminAuth(w, r, tmpl, "/admin/lists") {
		return
	}

	data := adminPageData{AdminAuthed: true}

	if r.Method == http.MethodPost {
		switch r.FormValue("admin_action") {
		case "create_list":
			listAddr := strings.TrimSpace(r.FormValue("list_address"))
			name := strings.TrimSpace(r.FormValue("name"))
			owner := strings.TrimSpace(r.FormValue("owner_email"))
			if err := ah.store.DB.CreateMailingList(listAddr, name, "", owner); err != nil {
				data.Error = "Failed to create list: " + err.Error()
			} else {
				data.Success = "List created"
			}
		case "delete_list":
			if err := ah.store.DB.DeleteMailingList(r.FormValue("list_address")); err != nil {
				data.Error = "Failed to delete list"
			} else {
				data.Success = "List deleted"
			}
		case "add_member":
			if err := ah.store.DB.AddListMember(r.FormValue("list_address"), strings.TrimSpace(r.FormValue("member_email"))); err != nil {
				data.Error = "Failed to add member"
			} else {
				data.Success = "Member added"
			}
		}
	}

	lists, _ := ah.store.DB.ListMailingLists()
	data.Lists = lists

	tmpl.render(w, "layout", pageData{Username: "Admin", Email: "admin", AdminData: data})
}

// handleAdminAuth checks admin authentication, returns true if authenticated.
func (ah *AdminHandlers) handleAdminAuth(w http.ResponseWriter, r *http.Request, tmpl templateRenderer, redirectPath string) bool {
	if ah.cfg.AdminSecret == "" {
		http.Error(w, "Admin interface not configured (set BDS_ADMIN_SECRET)", http.StatusForbidden)
		return false
	}
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
				http.Redirect(w, r, redirectPath, http.StatusSeeOther)
				return false
			}
			tmpl.render(w, "layout", pageData{Error: "Invalid admin secret"})
			return false
		}
		tmpl.render(w, "layout", pageData{})
		return false
	}
	return true
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
