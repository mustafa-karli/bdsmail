package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mustafakarli/bdsmail/internal/store"
)

// isSuperAdmin checks if user has superadmin permission.
func (h *Handlers) isSuperAdmin(email string) bool {
	ok, _ := h.store.DB.HasPermission(email, "superadmin")
	return ok
}

// isPlatformHost checks if the request is to the platform domain.
func (h *Handlers) isPlatformHost(r *http.Request) bool {
	host := r.Host
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host == h.cfg.MailHostname
}

// HandleSuperAdmin renders the super admin dashboard.
func (h *Handlers) HandleSuperAdmin(w http.ResponseWriter, r *http.Request, tmpl templateRenderer) {
	if !h.isPlatformHost(r) {
		http.Error(w, "Not available on this domain", http.StatusForbidden)
		return
	}
	email, ok := h.requireAuth(w, r)
	if !ok {
		return
	}
	if !h.isSuperAdmin(email) {
		http.Error(w, "Super admin access required", http.StatusForbidden)
		return
	}

	pd := h.userPageData(email)

	if r.Method == http.MethodPost {
		action := r.FormValue("action")
		switch action {
		case "suspend_domain":
			domain := r.FormValue("domain")
			h.store.DB.UpdateDomainStatus(domain, "", "")
			pd.Success = "Domain " + domain + " suspended"
		case "delete_domain":
			domain := r.FormValue("domain")
			h.store.DB.DeleteDNSRecords(domain)
			h.store.DB.DeleteDomain(domain)
			h.store.RefreshDomains()
			pd.Success = "Domain " + domain + " deleted"
		}
	}

	domains, _ := h.store.DB.ListDomains()
	users, _ := h.store.DB.ListUsers()

	type domainView struct {
		Name      string
		Status    string
		CreatedBy string
		UserCount int
	}

	// Count users per domain
	domainUserCount := make(map[string]int)
	for _, u := range users {
		domainUserCount[u.Domain]++
	}

	var dv []domainView
	for _, d := range domains {
		dv = append(dv, domainView{
			Name:      d.Name,
			Status:    d.Status,
			CreatedBy: d.CreatedBy,
			UserCount: domainUserCount[d.Name],
		})
	}

	pd.AdminData = dv
	pd.OAuthNonce = "superadmin"
	tmpl.render(w, "layout", pd)
}

// --- API ---

func (h *Handlers) APISuperAdminDomains(w http.ResponseWriter, r *http.Request) {
	if !h.isPlatformHost(r) {
		jsonError(w, 403, "Not available on this domain")
		return
	}
	email := getSessionUser(r, h.sessions)
	if email == "" || !h.isSuperAdmin(email) {
		jsonError(w, 403, "Super admin access required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		domains, _ := h.store.DB.ListDomains()
		jsonOK(w, domains)

	case http.MethodDelete:
		var body struct {
			Domain string `json:"domain"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		h.store.DB.DeleteDNSRecords(body.Domain)
		h.store.DB.DeleteDomain(body.Domain)
		h.store.RefreshDomains()
		jsonOK(w, map[string]string{"status": "ok"})
	}
}

func (h *Handlers) APISuperAdminUsers(w http.ResponseWriter, r *http.Request) {
	if !h.isPlatformHost(r) {
		jsonError(w, 403, "Not available on this domain")
		return
	}
	email := getSessionUser(r, h.sessions)
	if email == "" || !h.isSuperAdmin(email) {
		jsonError(w, 403, "Super admin access required")
		return
	}

	users, _ := h.store.DB.ListUsers()
	type uv struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Domain      string `json:"domain"`
		Status      string `json:"status"`
	}
	var result []uv
	for _, u := range users {
		result = append(result, uv{u.Email(), u.DisplayName, u.Domain, u.Status})
	}
	if result == nil {
		result = []uv{}
	}
	jsonOK(w, result)
}

// Ensure store import used
var _ = store.SplitEmail
